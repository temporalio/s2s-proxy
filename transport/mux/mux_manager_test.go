package mux

import (
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/config"
)

// helper to create a SessionWithConn backed by net.Pipe
func newSessionWithConn(t *testing.T) (swc *SessionWithConn, remote net.Conn, cleanup func()) {
	t.Helper()
	c1, c2 := net.Pipe()
	sess, err := yamux.Client(c1, nil)
	assert.NoError(t, err)
	swc = &SessionWithConn{session: sess, conn: c1}
	cleanup = func() {
		_ = sess.Close()
		_ = c1.Close()
		_ = c2.Close()
	}
	return swc, c2, cleanup
}

func TestWithConnection_SkipsClosedSessionsAndWaitsForNew(t *testing.T) {
	logger := log.NewTestLogger()
	mgr := NewMuxManager(config.MuxTransportConfig{Name: "test"}, logger)

	// Wait for a connection
	receivedConn := make(chan struct{})
	var err error
	go func() {
		err = mgr.WithConnection(func(s *SessionWithConn) error {
			close(receivedConn)
			return nil
		})
	}()
	assert.NoError(t, err, "unexpected error")

	// Give goroutine time to enter wait
	time.Sleep(20 * time.Millisecond)

	// provide a closed connection
	invalidSWC, _, cleanupClosed := newSessionWithConn(t)
	cleanupClosed()
	mgr.ReplaceConnection(invalidSWC)

	select {
	case <-receivedConn:
		t.Fatal("WithConnection should not have proceeded with closed session")
	default:
	}

	// Provide a valid connection
	openSWC, _, cleanupOpen := newSessionWithConn(t)
	defer cleanupOpen()
	mgr.ReplaceConnection(openSWC)

	select {
	case <-receivedConn:
		// success
	case <-time.After(2 * time.Second):
		t.Fatal("WithConnection did not proceed after open session was provided")
	}
}

func TestWithConnection_ReleasesOnShutdown(t *testing.T) {
	logger := log.NewTestLogger()
	mgr := NewMuxManager(config.MuxTransportConfig{Name: "test"}, logger)

	// Put an open connection in place
	swc, _, cleanup := newSessionWithConn(t)
	defer cleanup()
	mgr.ReplaceConnection(swc)

	// Start a waiter; it should be waiting because WithConnection always waits before checking
	errCh := make(chan error, 1)
	go func() {
		errCh <- mgr.WithConnection(func(s *SessionWithConn) error { return nil })
	}()

	// Give goroutine time to enter wait
	time.Sleep(20 * time.Millisecond)

	// Shutdown should close existing connection and wake waiter with an error
	mgr.ShutDown()

	// Verify the existing session is closed
	assert.Eventually(t, func() bool { return swc.session.IsClosed() }, time.Second, 10*time.Millisecond)

	// Waiter should return shutdown error
	select {
	case err := <-errCh:
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "shutting down")
	case <-time.After(2 * time.Second):
		t.Fatal("WithConnection waiter did not unblock on shutdown")
	}
}

// passthroughConnProvider stores a connection and provides it repeatedly. Blocks on connAvailable so the test can control when it fires
type passthroughConnProvider struct {
	conn          net.Conn
	connAvailable chan struct{}
}

func (p *passthroughConnProvider) NewConnection() (net.Conn, error) {
	<-p.connAvailable
	return p.conn, nil
}
func (p *passthroughConnProvider) CloseProvider() {
	_ = p.conn.Close()
}

// connWaiter repeatedly waits on mgr.WithConnection, and then sends the session to connSeen. Stops when it sees shutDown
type connWaiter struct {
	shutDown chan struct{}
	connSeen chan *SessionWithConn
	mgr      *MuxManager
}

func (c connWaiter) Start() {
	go func() {
		for {
			select {
			case <-c.shutDown:
				return
			default:
				_ = c.mgr.WithConnection(func(s *SessionWithConn) error {
					c.connSeen <- s
					<-s.session.CloseChan()
					return nil
				})
			}
		}
	}()
}

func TestWithConnection_MuxProviderReconnect(t *testing.T) {
	logger := log.NewTestLogger()

	clientConn, serverConn := net.Pipe()
	clientConnProvider := passthroughConnProvider{conn: clientConn, connAvailable: make(chan struct{}, 1)}
	serverConnProvider := passthroughConnProvider{conn: serverConn, connAvailable: make(chan struct{}, 1)}
	clientConnProvider.connAvailable <- struct{}{}
	serverConnProvider.connAvailable <- struct{}{}

	_, clientConnWaiter, clientConnDisconnected, clientProvider := buildMuxReader("clientMux", &clientConnProvider, yamux.Client, logger)
	_, serverConnWaiter, serverConnDisconnected, serverProvider := buildMuxReader("serverMux", &serverConnProvider, yamux.Server, logger)

	// Avoid the MuxManager's Start(), which assumes we're using TCP
	serverProvider.Start()
	clientProvider.Start()

	clientSession := expectCh(t, clientConnWaiter.connSeen, 2*time.Second, "WithConnection should have seen a connection from the clientProvider")
	serverSession := expectCh(t, serverConnWaiter.connSeen, 2*time.Second, "WithConnection should have seen a connection from the serverProvider")

	// Close connections. We should see both sides fire disconnectFn
	_ = clientConn.Close()
	expectCh(t, clientConnDisconnected, 2*time.Second, "Client connection failed to disconnect")
	_ = serverConn.Close()
	expectCh(t, serverConnDisconnected, 2*time.Second, "Server connection failed to disconnect")

	assert.True(t, clientSession.IsClosed(), "clientSession should be closed")
	assert.True(t, serverSession.IsClosed(), "serverSession should be closed")

	expectNoCh(t, clientConnWaiter.connSeen, 50*time.Millisecond, "Should not have seen a client conn while disconnected")
	expectNoCh(t, serverConnWaiter.connSeen, 50*time.Millisecond, "Should not have seen a client conn while disconnected")

	clientConn, serverConn = net.Pipe()
	clientConnProvider.conn = clientConn
	serverConnProvider.conn = serverConn
	serverConnProvider.connAvailable <- struct{}{}
	clientConnProvider.connAvailable <- struct{}{}

	expectCh(t, clientConnWaiter.connSeen, 2*time.Second, "WithConnection should have seen a new connection from the clientProvider")
	expectCh(t, serverConnWaiter.connSeen, 2*time.Second, "WithConnection should have seen a new connection from the clientProvider")
}

func buildMuxReader(name string, connProvider connProvider, yamuxFn func(io.ReadWriteCloser, *yamux.Config) (*yamux.Session, error), logger log.Logger) (*MuxManager, *connWaiter, chan struct{}, *MuxProvider) {
	mgr := NewMuxManager(config.MuxTransportConfig{Name: name}, logger)
	connDisconnected := make(chan struct{}, 1)
	provider := &MuxProvider{
		name:         name,
		connProvider: connProvider,
		sessionFn: func(conn net.Conn) (*yamux.Session, error) {
			//logger.Info("Server connected")
			return yamuxFn(conn, nil)
		},
		onDisconnectFn: func() {
			//logger.Info("Server disconnected")
			connDisconnected <- struct{}{}
		},
		setNewTransport: mgr.ReplaceConnection,
		metricLabels:    []string{"a", "b", "c"},
		logger:          logger,
		shutDown:        channel.NewShutdownOnce(),
		startOnce:       sync.Once{},
	}
	connWaiter := &connWaiter{shutDown: make(chan struct{}), connSeen: make(chan *SessionWithConn), mgr: mgr}
	connWaiter.Start()
	return mgr, connWaiter, connDisconnected, provider
}

func expectNoCh[T any](t *testing.T, ch <-chan T, timeout time.Duration, message string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatal(message)
	case <-time.After(timeout):
	}
}

func expectCh[T any](t *testing.T, ch chan T, timeout time.Duration, message string) T {
	t.Helper()
	select {
	case item := <-ch:
		return item
	case <-time.After(timeout):
		t.Fatal(message)
		// Never returned, but Go needs this
		var empty T
		return empty
	}
}
