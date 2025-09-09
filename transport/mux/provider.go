package mux

import (
	"net"
	"sync"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	// MuxProvider manages the process of opening a connection with connProvider, setting up a yamux Session with sessionFn,
	// and then reporting that session via setNewTransport. If the session closes, a new one will be created and notified
	// using setNewTransport. The actual logic for the connection providers are in establisher.go and receiver.go.
	muxProvider struct {
		name            string
		connProvider    connProvider
		sessionFn       func(net.Conn) (*yamux.Session, error)
		onDisconnectFn  func()
		setNewTransport SetTransportCallback
		metricLabels    []string
		logger          log.Logger
		shutDown        channel.ShutdownOnce
		startOnce       sync.Once
		startedCh       chan struct{}
		cleanedUpCh     chan struct{}
	}
	SetTransportCallback func(swc *SessionWithConn)
	// connProvider represents a way to get connections, either as a client or a server. MuxProvider guarantees that
	// Close is called when the provider exits
	connProvider interface {
		NewConnection() (net.Conn, error)
		CleanupCh() <-chan struct{}
	}
	MuxProvider interface {
		Start()
		CleanupCh() <-chan struct{}
		Stop()
	}
)

// NewMuxProvider creates a new custom MuxProvider with the provided data.
func NewMuxProvider(name string,
	connProvider connProvider,
	sessionFn func(net.Conn) (*yamux.Session, error),
	onDisconnectFn func(),
	setNewTransport SetTransportCallback,
	metricLabels []string,
	logger log.Logger,
	shutDown channel.ShutdownOnce,
) MuxProvider {
	return &muxProvider{
		name:            name,
		connProvider:    connProvider,
		sessionFn:       sessionFn,
		onDisconnectFn:  onDisconnectFn,
		setNewTransport: setNewTransport,
		metricLabels:    metricLabels,
		logger:          logger,
		shutDown:        shutDown,
		startOnce:       sync.Once{},
		startedCh:       make(chan struct{}),
		cleanedUpCh:     make(chan struct{}),
	}
}

func (m *muxProvider) CleanupCh() <-chan struct{} {
	return m.cleanedUpCh
}

// Start starts the MuxProvider. MuxProvider can only be started once, and once they are started they will run until
// the provided context is cancelled. The MuxProvider will cancel the context itself if it exits due to an unrecoverable
// error or panic. Connection instability is not unrecoverable: the MuxProvider will detect yamux Session exit and open
// a new session.
func (m *muxProvider) Start() {
	m.startOnce.Do(func() {
		close(m.startedCh)
		var err error
		go func() {
			defer func() {
				m.logger.Info("muxProvider shutting down", tag.NewErrorTag("lastError", err))
				// It's not expected that this goroutine will panic, but if it does, make sure Shutdown runs
				m.shutDown.Shutdown()
				m.setNewTransport(nil)
				<-m.connProvider.CleanupCh()
				select {
				case <-m.cleanedUpCh:
				default:
					close(m.cleanedUpCh)
				}
			}()
		connect:
			for {
				if m.shutDown.IsShutdown() {
					return
				}
				m.logger.Info("mux session watcher starting")

				var conn net.Conn
				conn, err = m.connProvider.NewConnection()
				if err != nil {
					continue connect
				}

				var session *yamux.Session
				session, err = m.sessionFn(conn)
				// Force Yamux to actually send something on the conn to make sure it's alive
				_, err = session.Ping()
				if err != nil {
					m.setNewTransport(nil)
					if !m.shutDown.IsShutdown() {
						m.logger.Error("got an invalid connection from connProvider", tag.Error(err))
						metrics.MuxErrors.WithLabelValues(m.metricLabels...).Inc()
					}
					continue connect
				}
				go observeYamuxSession(session, observerLabels(session.LocalAddr().String(), session.RemoteAddr().String(), "conn", m.name))

				m.setNewTransport(&SessionWithConn{Session: session, Conn: conn})
				metrics.MuxConnectionEstablish.WithLabelValues(m.metricLabels...).Inc()
				// Wait for shutdown or session reconnect
				select {
				case <-session.CloseChan():
				case <-m.shutDown.Channel():
				}
				m.onDisconnectFn()
			}
		}()
	})
}

// Stop stops the MuxProvider and its associated connProvider permanently.
func (m *muxProvider) Stop() {
	m.shutDown.Shutdown()
	select {
	case <-m.startedCh:
	default:
		// If we were stopped before starting for some reason, close our state channels to unblock anything waiting
		close(m.startedCh)
		close(m.cleanedUpCh)
		// Ensure Start() is blocked too
		m.startOnce.Do(func() {})
	}
}
