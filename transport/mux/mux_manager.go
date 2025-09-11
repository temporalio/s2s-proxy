package mux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/metrics"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

type (
	// MuxManager is the interface between an asynchronous MuxProvider and some number of readers that need to
	// access a yamux session. The underlying MuxProvider will continuously reestablish a usable yamux session, which
	// can be retrieved by readers using WithConnection and TryConnectionOrElse.
	muxManager struct {
		config         config.MuxTransportConfig
		metricLabels   []string                        // Derived from the config
		muxProvider    MuxProvider                     // A reference to the MuxProvider that provides myxConnection
		muxConnection  atomic.Pointer[SessionWithConn] // Underlying mux value. This starts as nil, and is set by the provider.
		connAvailable  sync.Cond                       // Condition lock for muxConnection. Used to notify threads waiting in WithConnection
		init           sync.Once                       // Ensures a MuxManager can only be started once
		shouldShutDown channel.ShutdownOnce            // Notify that muxManager should shut down
		hasShutDown    channel.ShutdownOnce            // Notify that muxManager has finished shutting down
		wakeInterval   time.Duration                   // wakeInterval sets the rate at which threads waiting on WithConnection are woken to check for context timeout
		logger         log.Logger
	}
	SessionWithConn struct {
		Session *yamux.Session
		Conn    net.Conn
	}
	MuxManager interface {
		// ConfigureMuxManager parses the provided MuxTransportConfig to set the appropriate MuxProvider.
		// It will return error if the config was invalid, for example if the TLS settings were not set properly.
		ConfigureMuxManager() error
		// Start starts the underlying MuxProvider
		Start()
		// Close closes the internal shutdown latch and sets the connection to nil. This will also stop the connection provider.
		// If you want to reopen connections you'll need to create a new MuxManager instance.
		Close()
		IsClosed() bool
		CloseChan() <-chan struct{}

		// TryConnectionOrElse grabs whatever connection is available and runs f on that connection. If the connection
		// is nil, it will return your orElse value.
		// The received SessionWithConn is guaranteed to be either a valid yamux session or a closed yamux session
		TryConnectionOrElse(f func(*SessionWithConn) any, orElse any) any
		// WithConnection waits for an open session, then runs the provided function with that session.
		// If the provided context is canceled or MuxManager is shut down while waiting, WithConnection will return nil+error.
		WithConnection(ctx context.Context, f func(*SessionWithConn) (any, error)) (result any, err error)
		// ReplaceConnection sets the available session and connection on MuxManager directly. It will notify any
		// threads waiting in WithConnection
		ReplaceConnection(swc *SessionWithConn)
		// PingSession runs yamux.Session.Ping(ctx) on the connection, if available. This helper method is identical
		// to just calling WithConnection(sessionWithConn.Session.Ping()) yourself. Useful for health checks on the
		// underlying connection.
		PingSession(ctx context.Context) error

		// Connect is part of the ClientTransport interface. This is used to establish an outbound grpc client.
		// It needs to create a dialer, dial the remote host using the provided mux, and then return the new connection.
		// This will block until a session is available.
		Connect(clientMetrics *grpcprom.ClientMetrics) (*grpc.ClientConn, error)
		Serve(server *grpc.Server) error
	}
)

func (s *SessionWithConn) IsClosed() bool {
	return s.Session.IsClosed()
}

// NewMuxManager constructs a MuxManager: an interface that provides an asynchronous MuxProvider that sets a yamux session
// and a single point where many threads can access that yamux session. The underlying MuxProvider will continuously
// reestablish a mux session, which is provided from MuxManager.WithConnection and MuxManager.TryConnectionOrElse
func NewMuxManager(cfg config.MuxTransportConfig, logger log.Logger) MuxManager {
	muxMgr := &muxManager{
		config:         cfg,
		muxConnection:  atomic.Pointer[SessionWithConn]{}, // WaitableValue
		connAvailable:  sync.Cond{L: &sync.Mutex{}},
		init:           sync.Once{},
		logger:         log.With(logger, tag.NewStringTag("component", "MuxManager")),
		shouldShutDown: channel.NewShutdownOnce(),
		hasShutDown:    channel.NewShutdownOnce(),
		wakeInterval:   time.Second * 5,
	}
	return muxMgr
}

func (m *muxManager) Close() {
	// shouldShutDown will notify the underlying provider that it's time to shut down
	m.shouldShutDown.Shutdown()
	// This Close() blocks until the provider is closed
	m.muxProvider.Close()
	// Make sure the connection pointer is emptied before finishing shutdown
	m.ReplaceConnection(nil)
	// Notify shutdown
	m.hasShutDown.Shutdown()
}

func (m *muxManager) CloseChan() <-chan struct{} {
	return m.hasShutDown.Channel()
}

func (m *muxManager) IsClosed() bool {
	return m.hasShutDown.IsShutdown()
}

func (m *muxManager) PingSession(ctx context.Context) error {
	_, err := m.WithConnection(ctx, func(swc *SessionWithConn) (any, error) {
		return swc.Session.Ping()
	})
	return err
}

func (m *muxManager) Connect(clientMetrics *grpcprom.ClientMetrics) (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		res, err := m.WithConnection(ctx, func(conn *SessionWithConn) (any, error) { return conn.Session.Open() })
		conn, ok := res.(net.Conn)
		if !ok {
			return nil, err
		}
		return conn, err
	}

	// Set hostname to unused since custom dialer is used.
	return grpcutil.Dial("unused", nil, clientMetrics, dialer)
}

func (m *muxManager) Serve(server *grpc.Server) error {
	_, err := m.WithConnection(context.Background(), func(s *SessionWithConn) (any, error) {
		return struct{}{}, server.Serve(s.Session)
	})
	return err
}

// SetCustomWakeInterval sets the speed at which the MuxProvider wakes waiting threads.
// Must be set BEFORE starting, intended for unit tests.
func SetCustomWakeInterval(m MuxManager, wakeInterval time.Duration) {
	if mm, ok := m.(*muxManager); ok {
		mm.wakeInterval = wakeInterval
	}
}

func (m *muxManager) WithConnection(ctx context.Context, f func(*SessionWithConn) (any, error)) (result any, err error) {
	// Some reminders on condition variables: A condition variable is a multi-layered lock: First we lock this
	// outer lock, which protects an internal semaphore/waitgroup. That semaphore/waitgroup represents a queue of waiting threads.
	metrics.MuxWaitingConnections.WithLabelValues(m.metricLabels...).Inc()
	defer metrics.MuxWaitingConnections.WithLabelValues(m.metricLabels...).Dec()
	m.connAvailable.L.Lock()
	for {
		// Check conditions first: If a valid connection is available, no need to wait
		if ctx.Err() != nil {
			m.connAvailable.L.Unlock()
			m.logger.Warn("Context canceled while trying to get connection", tag.Error(ctx.Err()))
			return result, ctx.Err()
		}
		if m.shouldShutDown.IsShutdown() {
			m.connAvailable.L.Unlock()
			return result, errors.New("the mux manager is shutting down")
		}
		// We want to see muxConnection is available and non-nil
		if ptr := m.muxConnection.Load(); ptr != nil && !ptr.IsClosed() {
			// Don't keep lock held while running f so that other code can use the connection
			m.connAvailable.L.Unlock()
			result, err = f(ptr)
			if err != nil {
				return result, fmt.Errorf("the provided function threw error %w", err)
			}
			metrics.MuxConnectionProvided.WithLabelValues(m.metricLabels...).Inc()
			return
		}
		// When we wait here, we add ourselves to the list of threads that should be restarted, let go of connAvailable.L,
		// and then suspend indefinitely. We will only be woken by Broadcast (wakes up every thread) or Signal (wakes up one thread)
		// As part of waking up from connAvailable.Wait, this thread will re-take connAvailable.L
		m.connAvailable.Wait()
	}

}

func (m *muxManager) TryConnectionOrElse(f func(*SessionWithConn) any, orElse any) any {
	conn := m.muxConnection.Load()
	if conn == nil {
		return orElse
	}
	return f(conn)
}

func (m *muxManager) ReplaceConnection(swc *SessionWithConn) {
	// Waiting on connAvailable.L here ensures no other threads are in the process of figuring out they should be notified
	// when Broadcast runs.
	m.connAvailable.L.Lock()
	defer m.connAvailable.L.Unlock()
	// Make sure the existing conn is fully closed
	existingConn := m.muxConnection.Load()
	if existingConn != nil {
		_ = existingConn.Session.Close()
		_ = existingConn.Conn.Close()
	}
	// Now add new conn
	m.muxConnection.Store(swc)
	// Now notify
	m.connAvailable.Broadcast()
}

func (m *muxManager) Start() {
	m.init.Do(func() {
		// Start a monitor that will periodically Broadcast so that waiting threads can check their contexts
		go func() {
			wakeThreads := time.NewTicker(m.wakeInterval)
			defer wakeThreads.Stop()
			for {
				select {
				case <-wakeThreads.C:
					m.connAvailable.Broadcast()
				case <-m.shouldShutDown.Channel():
					m.connAvailable.Broadcast()
					return
				}
			}
		}()
		// Start the mux provider
		m.muxProvider.Start()
	})
}

func (m *muxManager) ConfigureMuxManager() error {
	var err error
	switch m.config.Mode {
	case config.ClientMode:
		m.logger.Info(fmt.Sprintf("Applying ClientMode mux provider from config: %v", m.config.Client))
		metricLabels := []string{m.config.Client.ServerAddress,
			string(m.config.Mode),
			m.config.Name,
		}
		m.muxProvider, err = NewMuxEstablisherProvider(m.config.Name, m.ReplaceConnection, m.config.Client, metricLabels, m.logger, m.shouldShutDown)
	case config.ServerMode:
		m.logger.Info(fmt.Sprintf("Applying ServerMode mux provider from config: %v", m.config.Server))
		metricLabels := []string{m.config.Server.ListenAddress,
			string(m.config.Mode),
			m.config.Name,
		}
		m.muxProvider, err = NewMuxReceiverProvider(m.config.Name, m.ReplaceConnection, m.config.Server, metricLabels, m.logger, m.shouldShutDown)
	default:
		return fmt.Errorf("invalid multiplexed transport mode: name %s, mode %s", m.config.Name, m.config.Mode)
	}
	return err
}
