package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
)

type (
	// StartManagedComponentFn describes a function that can start a GoRoutine with the provided Session. The component
	// is expected to monitor shutdown and exit when requested.
	StartManagedComponentFn func(lifetime context.Context, sessionId string, session *yamux.Session)

	MuxSessionState int
	MuxSessionInfo  struct {
		State MuxSessionState
		Err   error
	}

	muxSession struct {
		lifetime context.Context
		cancel   context.CancelFunc
		id       string
		state    atomic.Pointer[MuxSessionInfo]
		session  *yamux.Session
		conn     net.Conn
	}
	ManagedMuxSession interface {
		IsClosed() bool
		// Close will ensure the underlying transports and components are closed too
		Close()
		CloseChan() <-chan struct{}
		Open() (net.Conn, error)
		State() *MuxSessionInfo
		Describe() string
	}
)

const (
	Connected MuxSessionState = iota
	Closed
	Error
)

var ErrClosedSession = errors.New("session closed")

func NewSession(lifetime context.Context, cancel context.CancelFunc, id string, session *yamux.Session, conn net.Conn, builders []StartManagedComponentFn, afterShutdown func()) ManagedMuxSession {
	s := &muxSession{
		lifetime: lifetime,
		cancel:   cancel,
		id:       id,
		session:  session,
		state:    atomic.Pointer[MuxSessionInfo]{},
		conn:     conn,
	}
	s.state.Store(&MuxSessionInfo{State: Connected})
	go healthCheck(s)
	for i := range builders {
		builders[i](s.lifetime, s.id, s.session)
	}
	// The provided context can close, or the underlying mux can die. If either of these happen, make sure everything
	// closes together
	go waitAndCleanup(s, afterShutdown)
	return s
}
func waitAndCleanup(s *muxSession, afterShutdown func()) {
	select {
	case <-s.session.CloseChan():
		s.cancel()
	case <-s.lifetime.Done():
		_ = s.session.Close()
		_ = s.conn.Close()
		s.state.Store(&MuxSessionInfo{State: Closed})
	}
	afterShutdown()
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
	return s.lifetime.Err() != nil
}

func (s *muxSession) Close() {
	s.cancel()
}

func (s *muxSession) CloseChan() <-chan struct{} {
	return s.lifetime.Done()
}

func (s *muxSession) Open() (net.Conn, error) {
	if s.lifetime.Err() != nil {
		return nil, ErrClosedSession
	}
	return s.session.Open()
}

func (s *muxSession) State() *MuxSessionInfo {
	return s.state.Load()
}

// net.Listener

func (s *muxSession) Accept() (net.Conn, error) {
	if s.lifetime.Err() != nil {
		return nil, ErrClosedSession
	}
	return s.session.Accept()
}
func (s *muxSession) Addr() net.Addr {
	return s.session.Addr()
}
func (s *muxSession) Describe() string {
	return fmt.Sprintf("[muxSession %s, state=%v, address=%s]", s.id, s.state.Load(), s.conn.RemoteAddr().String())
}
