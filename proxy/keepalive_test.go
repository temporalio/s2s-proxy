package proxy

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"golang.org/x/net/http2"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/logging"
)

// These tests talk raw HTTP/2 to the proxy's gRPC server, or act as a raw HTTP/2 server for the proxy's gRPC client,
// so they can send and observe PING frames directly. A real gRPC client can't ping more often than every 10s.

func TestServerKeepaliveEnforcementPolicy(t *testing.T) {
	t.Run("default policy rejects frequent pings", func(t *testing.T) {
		t.Parallel()
		peer := dialRawHTTP2(t, startKeepaliveTestServer(t, nil), true)
		peer.sendPings(t, 5, 50*time.Millisecond)

		require.Eventually(t, peer.isClosed, 5*time.Second, 10*time.Millisecond, "server should close the connection")
		code, debug := peer.goAway()
		require.Equal(t, http2.ErrCodeEnhanceYourCalm, code)
		require.Equal(t, "too_many_pings", debug)
	})

	t.Run("configured policy accepts frequent pings", func(t *testing.T) {
		t.Parallel()
		addr := startKeepaliveTestServer(t, &config.ServerKeepaliveConfig{
			EnforcementPolicy: &config.KeepaliveEnforcementPolicyConfig{
				MinTime:             10 * time.Millisecond,
				PermitWithoutStream: true,
			},
		})
		peer := dialRawHTTP2(t, addr, true)
		peer.sendPings(t, 5, 50*time.Millisecond)

		// Give the server time to answer the last ping, or to reject it.
		time.Sleep(200 * time.Millisecond)
		require.False(t, peer.isClosed(), "server should keep the connection open")
		code, _ := peer.goAway()
		require.Equal(t, http2.ErrCode(0), code, "server should not send GOAWAY")
		require.EqualValues(t, 5, peer.pingAcks.Load())
	})
}

func TestServerKeepalivePingsSilentClient(t *testing.T) {
	t.Parallel()
	// grpc-go raises a server Time below 1s to 1s.
	addr := startKeepaliveTestServer(t, &config.ServerKeepaliveConfig{Time: time.Second, Timeout: time.Second})
	// This client never answers pings, like a peer whose connection died without a FIN or RST.
	peer := dialRawHTTP2(t, addr, false)

	require.Eventually(t, peer.isClosed, 10*time.Second, 50*time.Millisecond,
		"server should close a connection whose client doesn't answer its ping")
	require.GreaterOrEqual(t, peer.pings.Load(), int32(1), "server should have pinged the client")
}

func TestClientKeepalivePingsServer(t *testing.T) {
	if testing.Short() {
		t.Skip("grpc-go won't ping more often than every 10s")
	}
	t.Parallel()
	server := startRawHTTP2Server(t)

	conn, err := createClient(t.Context(), "keepalive-test", config.ClusterDefinition{
		ConnectionType: config.ConnTypeTCP,
		TcpClient:      config.TCPTLSInfo{ConnectionString: server.addr},
		Keepalive: &config.KeepaliveConfig{Client: &config.ClientKeepaliveConfig{
			Time:                config.MinClientKeepaliveTime,
			Timeout:             5 * time.Second,
			PermitWithoutStream: true,
		}},
	}, "inbound", auth.EmptyCredentialProvider{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	conn.(describableClientConn).Connect()

	require.Eventually(t, func() bool { return server.pings.Load() > 0 }, 15*time.Second, 100*time.Millisecond,
		"client should ping an idle connection")
}

// startKeepaliveTestServer starts a proxy gRPC server built by makeServerOptions with ka and returns its address.
func startKeepaliveTestServer(t *testing.T, ka *config.ServerKeepaliveConfig) string {
	t.Helper()
	nsTranslations, err := (&config.StringTranslator{}).AsLocalToRemoteBiMap()
	require.NoError(t, err)
	opts, err := makeServerOptions(serverConfiguration{
		name:           "keepalive-test",
		directionLabel: "outbound",
		clusterDefinition: config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			Keepalive:      &config.KeepaliveConfig{Server: ka},
		},
		nsTranslations: nsTranslations,
		loggers:        logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{})),
	}, encryption.TLSConfig{})
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := grpc.NewServer(opts...)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return listener.Addr().String()
}

// rawHTTP2Client is an HTTP/2 client connection with no streams that records what the server sends.
type rawHTTP2Client struct {
	conn     net.Conn
	writeMu  sync.Mutex
	framer   *http2.Framer
	ackPings bool

	pings    atomic.Int32 // PINGs the server sent
	pingAcks atomic.Int32 // answers to our PINGs
	closed   atomic.Bool

	goAwayMu    sync.Mutex
	goAwayCode  http2.ErrCode
	goAwayDebug string
}

func dialRawHTTP2(t *testing.T, addr string, ackPings bool) *rawHTTP2Client {
	t.Helper()
	conn, err := net.Dial("tcp", addr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = conn.Write([]byte(http2.ClientPreface))
	require.NoError(t, err)
	c := &rawHTTP2Client{conn: conn, framer: http2.NewFramer(conn, conn), ackPings: ackPings}
	require.NoError(t, c.framer.WriteSettings())
	go c.readLoop()
	return c
}

func (c *rawHTTP2Client) readLoop() {
	defer c.closed.Store(true)
	for {
		frame, err := c.framer.ReadFrame()
		if err != nil {
			return
		}
		switch f := frame.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				c.write(func() error { return c.framer.WriteSettingsAck() })
			}
		case *http2.PingFrame:
			if f.IsAck() {
				c.pingAcks.Add(1)
				continue
			}
			c.pings.Add(1)
			if c.ackPings {
				data := f.Data
				c.write(func() error { return c.framer.WritePing(true, data) })
			}
		case *http2.GoAwayFrame:
			c.goAwayMu.Lock()
			if c.goAwayCode == 0 {
				c.goAwayCode = f.ErrCode
				c.goAwayDebug = string(f.DebugData())
			}
			c.goAwayMu.Unlock()
		}
	}
}

func (c *rawHTTP2Client) write(fn func() error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	_ = fn()
}

func (c *rawHTTP2Client) sendPings(t *testing.T, n int, interval time.Duration) {
	t.Helper()
	for i := range n {
		c.write(func() error { return c.framer.WritePing(false, [8]byte{byte(i + 1)}) })
		time.Sleep(interval)
	}
}

func (c *rawHTTP2Client) isClosed() bool { return c.closed.Load() }

func (c *rawHTTP2Client) goAway() (http2.ErrCode, string) {
	c.goAwayMu.Lock()
	defer c.goAwayMu.Unlock()
	return c.goAwayCode, c.goAwayDebug
}

// rawHTTP2Server accepts HTTP/2 connections, answers SETTINGS and PING, and counts the PINGs it receives.
type rawHTTP2Server struct {
	addr  string
	pings atomic.Int32
}

func startRawHTTP2Server(t *testing.T) *rawHTTP2Server {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	s := &rawHTTP2Server{addr: listener.Addr().String()}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = conn.Close() })
			go s.serve(conn)
		}
	}()
	return s
}

func (s *rawHTTP2Server) serve(conn net.Conn) {
	preface := make([]byte, len(http2.ClientPreface))
	if _, err := io.ReadFull(conn, preface); err != nil {
		return
	}
	framer := http2.NewFramer(conn, conn)
	if err := framer.WriteSettings(); err != nil {
		return
	}
	for {
		frame, err := framer.ReadFrame()
		if err != nil {
			return
		}
		switch f := frame.(type) {
		case *http2.SettingsFrame:
			if !f.IsAck() {
				_ = framer.WriteSettingsAck()
			}
		case *http2.PingFrame:
			if !f.IsAck() {
				s.pings.Add(1)
				_ = framer.WritePing(true, f.Data)
			}
		}
	}
}
