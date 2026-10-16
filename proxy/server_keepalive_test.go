package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
)

// keepaliveTestClientParams pings as often as grpc-go allows (it clamps Time to at least 10s),
// including on idle connections, like a Temporal server with client keepalive enabled.
var keepaliveTestClientParams = keepalive.ClientParameters{
	Time:                10 * time.Second,
	Timeout:             5 * time.Second,
	PermitWithoutStream: true,
}

// keepaliveTestObservationWindow is long enough for grpc-go's default enforcement to reject
// the client: the first ping is free, the next three are strikes, and the third strike
// (about 40s in) triggers GOAWAY too_many_pings.
const keepaliveTestObservationWindow = 55 * time.Second

func TestProxyServerAcceptsClientKeepalivePings(t *testing.T) {
	if testing.Short() {
		t.Skip("observes an idle connection for about a minute")
	}

	t.Run("default enforcement closes the idle client", func(t *testing.T) {
		t.Parallel()
		listener, err := net.Listen("tcp", "localhost:0")
		require.NoError(t, err)
		server := grpc.NewServer()
		go func() { _ = server.Serve(listener) }()
		t.Cleanup(server.Stop)

		lost, goAway := observeKeepaliveIdleConnection(t, listener.Addr().String())

		// This subtest exercises grpc-go itself, not proxy code. It proves the observation window
		// is long enough to catch too_many_pings. If grpc-go changes its default enforcement,
		// update the window rather than deleting this check.
		require.True(t, lost, "grpc-go's default enforcement should drop the pinging idle client within the observation window")
		require.NotNil(t, goAway, "expected the server to send GOAWAY")
		require.True(t, goAway.isTooManyPings(), "expected GOAWAY ENHANCE_YOUR_CALM \"too_many_pings\", got %v %q",
			goAway.code, goAway.debugData)
	})

	t.Run("proxy outbound server keeps the idle client connected", func(t *testing.T) {
		t.Parallel()
		loggerProvider := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
		plcc := newPairedLocalClusterConnection(t, false, loggerProvider)
		plcc.StartAll(t)

		// The local proxy's outbound server is the listener the local Temporal server dials.
		lost, goAway := observeKeepaliveIdleConnection(t, plcc.addresses.localProxyOutbound)

		if goAway != nil {
			require.False(t, goAway.isTooManyPings(),
				"the proxy server should not reject a client pinging every 10s, got GOAWAY %v %q", goAway.code, goAway.debugData)
		}
		require.False(t, lost, "the proxy server should keep the pinging idle client connected")
	})
}

// observeKeepaliveIdleConnection dials addr with keepaliveTestClientParams, waits for the connection to become
// READY, then leaves it idle for keepaliveTestObservationWindow. It reports whether the
// connection left READY and the first GOAWAY the server sent, if any.
func observeKeepaliveIdleConnection(t *testing.T, addr string) (lost bool, goAway *keepaliveTestGoAway) {
	t.Helper()
	recorder := &keepaliveTestGoAwayRecorder{}
	conn, err := grpc.NewClient("passthrough:///"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepaliveTestClientParams),
		grpc.WithContextDialer(recorder.dial),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	conn.Connect()
	readyCtx, cancelReady := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelReady()
	for state := conn.GetState(); state != connectivity.Ready; state = conn.GetState() {
		require.True(t, conn.WaitForStateChange(readyCtx, state), "connection never became READY")
	}

	observeCtx, cancelObserve := context.WithTimeout(t.Context(), keepaliveTestObservationWindow)
	defer cancelObserve()
	// WaitForStateChange returns false only when the window ends with the state unchanged.
	lost = conn.WaitForStateChange(observeCtx, connectivity.Ready)
	if lost {
		// Give the frame parser a moment to record a GOAWAY that arrived just before the drop.
		require.Eventually(t, func() bool { return recorder.first() != nil }, time.Second, 10*time.Millisecond,
			"connection left READY without the server sending GOAWAY")
	}
	return lost, recorder.first()
}

type keepaliveTestGoAway struct {
	code      http2.ErrCode
	debugData string
}

func (g *keepaliveTestGoAway) isTooManyPings() bool {
	return g.code == http2.ErrCodeEnhanceYourCalm && g.debugData == "too_many_pings"
}

// keepaliveTestGoAwayRecorder dials plain TCP connections for the gRPC client and parses a copy of
// everything the server sends, recording the first GOAWAY frame. The gRPC client still
// reads the connection normally, so its transport and keepalive behavior are unchanged.
type keepaliveTestGoAwayRecorder struct {
	mu     sync.Mutex
	goAway *keepaliveTestGoAway
}

func (r *keepaliveTestGoAwayRecorder) first() *keepaliveTestGoAway {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.goAway
}

func (r *keepaliveTestGoAwayRecorder) dial(ctx context.Context, addr string) (net.Conn, error) {
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	pr, pw := io.Pipe()
	go r.parse(pr)
	return &keepaliveTestFrameTapConn{Conn: conn, tap: pw}, nil
}

// parse reads server-to-client frames. A server sends no connection preface, so the
// stream starts directly with frames.
func (r *keepaliveTestGoAwayRecorder) parse(pr *io.PipeReader) {
	framer := http2.NewFramer(io.Discard, pr)
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			// Keep draining so the tap never blocks the gRPC client's reads.
			_, _ = io.Copy(io.Discard, pr)
			return
		}
		if ga, ok := frame.(*http2.GoAwayFrame); ok {
			r.mu.Lock()
			if r.goAway == nil {
				// DebugData is only valid until the next ReadFrame, so copy it.
				r.goAway = &keepaliveTestGoAway{code: ga.ErrCode, debugData: string(ga.DebugData())}
			}
			r.mu.Unlock()
		}
	}
}

// keepaliveTestFrameTapConn copies every byte read from the connection into tap.
type keepaliveTestFrameTapConn struct {
	net.Conn
	tap *io.PipeWriter
}

func (c *keepaliveTestFrameTapConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 {
		_, _ = c.tap.Write(p[:n])
	}
	if err != nil {
		_ = c.tap.CloseWithError(err)
	}
	return n, err
}

func (c *keepaliveTestFrameTapConn) Close() error {
	_ = c.tap.CloseWithError(errors.New("connection closed"))
	return c.Conn.Close()
}
