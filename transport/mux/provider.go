package mux

import (
	"net"
	"sync"
	"sync/atomic"

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
		startOnce       sync.Once
		hasStarted      atomic.Bool
		shouldShutDown  channel.ShutdownOnce
		hasCleanedUp    channel.ShutdownOnce
	}
	SetTransportCallback func(swc *SessionWithConn)
	// connProvider represents a way to get connections, either as a client or a server. MuxProvider guarantees that
	// Close is called when the provider exits
	connProvider interface {
		NewConnection() (net.Conn, error)
		CloseCh() <-chan struct{}
	}
	MuxProvider interface {
		Start()
		Close()
		CloseCh() <-chan struct{}
		isClosed() bool
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
		startOnce:       sync.Once{},
		hasStarted:      atomic.Bool{},
		shouldShutDown:  shutDown,
		hasCleanedUp:    channel.NewShutdownOnce(),
	}
}

// Start starts the MuxProvider. MuxProvider can only be started once, and once they are started they will run until
// the provided context is cancelled. The MuxProvider will cancel the context itself if it exits due to an unrecoverable
// error or panic. Connection instability is not unrecoverable: the MuxProvider will detect yamux Session exit and open
// a new session.
func (m *muxProvider) Start() {
	m.startOnce.Do(func() {
		m.hasStarted.Store(true)
		var err error
		go func() {
			defer func() {
				m.logger.Info("muxProvider shutting down", tag.NewErrorTag("lastError", err))
				// If shouldShutDown wasn't already set, this goroutine exiting should force it
				m.shouldShutDown.Shutdown()
				m.setNewTransport(nil)
				// Wait for connProvider to finish cleanup before notifying
				<-m.connProvider.CloseCh()
				// Notify all resources are cleaned up
				m.hasCleanedUp.Shutdown()
			}()
		connect:
			for {
				if m.shouldShutDown.IsShutdown() {
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
					if !m.shouldShutDown.IsShutdown() {
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
				case <-m.shouldShutDown.Channel():
				}
				m.onDisconnectFn()
			}
		}()
	})
}

func (m *muxProvider) CloseCh() <-chan struct{} {
	return m.hasCleanedUp.Channel()
}

// Close stops the MuxProvider and its associated connProvider permanently.
func (m *muxProvider) Close() {
	m.shouldShutDown.Shutdown()
	// The connProvider has a thread that cleans up resources created by Start(), but if we never started that thread,
	// we need to consume m.startOnce, and then mark everything as cleaned up
	m.startOnce.Do(func() {})
	if !m.hasStarted.Load() {
		m.hasCleanedUp.Shutdown()
	}
	// Wait for connProvider to finish. This is closed from the goroutine started in Start()
	<-m.hasCleanedUp.Channel()
}
func (m *muxProvider) isClosed() bool {
	return m.hasCleanedUp.IsShutdown()
}
