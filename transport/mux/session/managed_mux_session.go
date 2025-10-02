package session

import (
	"errors"
	"net"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/channel"
)

type (
	// StartManagedComponentFn describes a function that can start a GoRoutine with the provided Session. The component
	// is expected to monitor shutdown and exit when requested.
	StartManagedComponentFn func(sessionId string, session *yamux.Session, shutdown channel.ShutdownOnce)

	MuxSessionState int
	MuxSessionInfo  struct {
		State MuxSessionState
		Err   error
	}

	muxSession struct {
		id       string
		state    atomic.Pointer[MuxSessionInfo]
		session  *yamux.Session
		conn     net.Conn
		shutDown channel.ShutdownOnce
	}
	ManagedMuxSession interface {
		IsClosed() bool
		// Close will ensure the underlying transports and components are closed too
		Close()
		CloseChan() <-chan struct{}
		Open() (net.Conn, error)
		State() *MuxSessionInfo
	}
)

const (
	Connected MuxSessionState = iota
	Closed
	Error
)

var ErrClosedSession = errors.New("session closed")

func NewSession(id string, session *yamux.Session, conn net.Conn, builders []StartManagedComponentFn, afterShutdown func()) ManagedMuxSession {
	s := &muxSession{
		id:       id,
		session:  session,
		state:    atomic.Pointer[MuxSessionInfo]{},
		conn:     conn,
		shutDown: channel.NewShutdownOnce(),
	}
	s.state.Store(&MuxSessionInfo{State: Connected})
	go healthCheck(s)
	for i := range builders {
		builders[i](s.id, s.session, s.shutDown)
	}
	// Close the ManagedMuxSession when the underlying transport closes
	go func() {
		<-s.session.CloseChan()
		s.Close()
		afterShutdown()
	}()
	return s
}
func healthCheck(s *muxSession) {
	for !s.session.IsClosed() {
		old := s.state.Load()
		_, err := s.session.Ping()
		state := Connected
		if err != nil {
			state = Error
		}
		// Ignore update if something else updated the state (i.e. connection closed)
		s.state.CompareAndSwap(old, &MuxSessionInfo{State: state, Err: err})
		select {
		case <-s.session.CloseChan():
		case <-time.After(time.Minute):
		}
	}
}

func (s *muxSession) IsClosed() bool {
	return s.shutDown.IsShutdown()
}

func (s *muxSession) Close() {
	s.state.Store(&MuxSessionInfo{State: Closed})
	s.shutDown.Shutdown()
	_ = s.session.Close()
	_ = s.conn.Close()
}

func (s *muxSession) CloseChan() <-chan struct{} {
	return s.shutDown.Channel()
}

func (s *muxSession) Open() (net.Conn, error) {
	if s.shutDown.IsShutdown() {
		return nil, ErrClosedSession
	}
	return s.session.Open()
}

func (s *muxSession) State() *MuxSessionInfo {
	return s.state.Load()
}

// net.Listener

func (s *muxSession) Accept() (net.Conn, error) {
	return s.session.Accept()
}
func (s *muxSession) Addr() net.Addr {
	return s.session.Addr()
}
