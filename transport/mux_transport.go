package transport

import (
	"context"
	"net"

	"github.com/hashicorp/yamux"
	"github.com/prometheus/client_golang/prometheus"
	"google.golang.org/grpc"
)

type muxTransportImpl struct {
	session *yamux.Session
	conn    net.Conn
	closeCh chan struct{} // if closed, means transport is closed (or disconnected).
}

func newMuxTransport(conn net.Conn, session *yamux.Session, labels prometheus.Labels) *muxTransportImpl {
	return &muxTransportImpl{
		conn:    conn,
		session: session,
		closeCh: make(chan struct{}),
	}
}

func (s *muxTransportImpl) Connect(metricLabels prometheus.Labels) (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return s.session.Open()
	}

	// Set hostname to unused since custom dialer is used.
	return dial("unused", nil, metricLabels, dialer)
}

func (s *muxTransportImpl) Serve(server *grpc.Server) error {
	return server.Serve(s.session)
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
	_ = s.conn.Close()
	_ = s.session.Close()
}

func (s *muxTransportImpl) close() {
	s.closeSession()
	// Wait for connection manager to notify close is completed.
	<-s.closeCh
}
