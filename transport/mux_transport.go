package transport

import (
	"context"
	"fmt"
	"net"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"google.golang.org/grpc"
)

type connAsListener struct {
	net.Conn
	closed bool
}

// Accept implements net.Listener.
func (c *connAsListener) Accept() (net.Conn, error) {
	if c.closed {
		return nil, fmt.Errorf("closed")
	}
	return c.Conn, nil
}

// Addr implements net.Listener.
func (c *connAsListener) Addr() net.Addr {
	return c.Conn.LocalAddr()
}

// Close implements net.Listener.
func (c *connAsListener) Close() error {
	c.closed = true
	return c.Conn.Close()
}

var _ net.Listener = (*connAsListener)(nil)

type muxTransportImpl struct {
	conn    *connAsListener
	closeCh chan struct{} // if closed, means transport is closed (or disconnected).
}

func newMuxTransport(conn *connAsListener) *muxTransportImpl {
	return &muxTransportImpl{
		conn:    conn,
		closeCh: make(chan struct{}),
	}
}

func (s *muxTransportImpl) Connect(clientMetrics *grpcprom.ClientMetrics) (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		if s.conn.closed {
			return nil, fmt.Errorf("closed")
		}
		return s.conn.Conn, nil
	}

	// Set hostname to unused since custom dialer is used.
	return dial("unused", nil, clientMetrics, dialer)
}

func (s *muxTransportImpl) Serve(server *grpc.Server) error {
	return server.Serve(s.conn)
}

func (m *muxTransportImpl) CloseChan() <-chan struct{} {
	return m.closeCh
}

func (m *muxTransportImpl) IsClosed() bool {
	select {
	case <-m.closeCh:
		return true
	default:
		return false
	}
}

func (s *muxTransportImpl) closeSession() {
	// _ = s.session.Close()
	_ = s.conn.Close()
}

func (s *muxTransportImpl) close() {
	s.closeSession()
	// Wait for connection manager to notify close is completed.
	<-s.closeCh
}
