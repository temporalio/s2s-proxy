package mux

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"golang.org/x/sync/semaphore"

	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	// MuxProvider manages the process of opening a connection with connProvider, setting up a yamux Session with sessionFn,
	// and then reporting that session via setNewTransport. If the session closes, a new one will be created and notified
	// using setNewTransport. The actual logic for the connection providers are in establisher.go and receiver.go.
	muxProvider struct {
		name         string
		connProvider connProvider
		sessionFn    func(net.Conn) (*yamux.Session, error)
		addNewMux    AddNewMux
		muxPermits   *semaphore.Weighted
		metricLabels []string
		logger       log.Logger
		startOnce    sync.Once
		hasStarted   atomic.Bool
		lifetime     context.Context
		hasCleanedUp channel.ShutdownOnce
	}
	AddNewMux func(*yamux.Session, net.Conn)
	// connProvider represents a way to get connections, either as a client or a server.
	connProvider interface {
		NewConnection() (net.Conn, error)
		CloseCh() <-chan struct{}
		Address() string
	}
	MuxProvider interface {
		Start()
		WaitForClose()
		CloseCh() <-chan struct{}
		isClosed() bool
		AllowMoreConns(amt int64)
		DrainConns(ctx context.Context, amt int64) error
		Address() string
		MetricLabels() []string
		HasConnectionsAvailable() bool
	}
)

// NewMuxProvider creates a new custom MuxProvider with the provided data.
func NewMuxProvider(ctx context.Context,
	name string,
	connProvider connProvider,
	sessionFn func(net.Conn) (*yamux.Session, error),
	muxCount int64,
	addNewMux AddNewMux,
	metricLabels []string,
	logger log.Logger,
) MuxProvider {
	// Initialize the counters so we get clear "0"s
	metrics.MuxConnectionEstablish.WithLabelValues(metricLabels...)
	metrics.MuxErrors.WithLabelValues(metricLabels...)
	provider := &muxProvider{
		name:         name,
		lifetime:     ctx,
		connProvider: connProvider,
		sessionFn:    sessionFn,
		muxPermits:   semaphore.NewWeighted(muxCount),
		addNewMux:    addNewMux,
		metricLabels: metricLabels,
		logger:       logger,
		startOnce:    sync.Once{},
		hasStarted:   atomic.Bool{},
		hasCleanedUp: channel.NewShutdownOnce(),
	}
	return provider
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
				m.logger.Info("muxProvider shutting down", tag.NewErrorTag("lastError", err), tag.NewStringTag("name", m.name))
				// Wait for connProvider to finish cleanup before notifying
				<-m.connProvider.CloseCh()
				// Notify all resources are cleaned up
				m.hasCleanedUp.Shutdown()
			}()
		connect:
			for {
				// Wait for shutdown or session needed
				err = m.muxPermits.Acquire(m.lifetime, 1)
				if err != nil {
					// context cancelled, no need to add a permit
					return
				}
				m.logger.Info("Getting new connection from connProvider")

				var conn net.Conn
				conn, err = m.connProvider.NewConnection()
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					m.muxPermits.Release(1)
					m.logger.Info("Error getting connection from connProvider", tag.Error(err))
					continue connect
				}

				var session *yamux.Session
				session, err = m.sessionFn(conn)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					m.muxPermits.Release(1)
					m.logger.Error("Failed to create mux session", tag.Error(err))
					continue connect
				}
				// Force Yamux to actually send something on the conn to make sure it's alive
				_, err = session.Ping()
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return
					}
					m.logger.Error("got an invalid connection from connProvider", tag.Error(err))
					metrics.MuxErrors.WithLabelValues(m.metricLabels...).Inc()
					m.muxPermits.Release(1)
					continue connect
				}

				m.addNewMux(session, conn)
				metrics.MuxConnectionEstablish.WithLabelValues(m.metricLabels...).Inc()
			}
		}()
	})
}

func (m *muxProvider) Address() string {
	return m.connProvider.Address()
}

func (m *muxProvider) MetricLabels() []string {
	return m.metricLabels
}

func (m *muxProvider) AllowMoreConns(amt int64) {
	m.muxPermits.Release(amt)
}

// DrainConns will block until the requested number of connections have drained.
// Warning: This method will not interfere with existing connections, it will just block new ones from being created.
func (m *muxProvider) DrainConns(ctx context.Context, amt int64) error {
	return m.muxPermits.Acquire(ctx, amt)
}

func (m *muxProvider) CloseCh() <-chan struct{} {
	return m.hasCleanedUp.Channel()
}

// WaitForClose does not actually close the provider. That should be done by closing the provider's context.
func (m *muxProvider) WaitForClose() {
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

// HasConnectionsAvailable checks whether more connections can be allocated by this provider.
// Used to return false to the inbound health check so that a receiver-provider can be removed from inbound VIP mappings
func (m *muxProvider) HasConnectionsAvailable() bool {
	if m.muxPermits.TryAcquire(1) {
		m.muxPermits.Release(1)
		return true
	}
	return false
}
