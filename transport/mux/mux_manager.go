package mux

import (
	"context"
	"errors"
	"fmt"
	"net"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	// MuxManager is the interface between an asynchronous MuxProvider and some number of readers that need to
	// access a yamux session. The underlying MuxProvider will continuously reestablish a usable yamux session, which
	// can be retrieved by readers using WithConnection and TryConnectionOrElse.
	muxManager struct {
		config        config.MuxTransportConfig
		muxConnection atomic.Pointer[SessionWithConn] // Underlying mux value. This starts as nil, and is set by the provider.
		connAvailable sync.Cond                       // Condition lock for muxConnection. Used to notify threads waiting in WithConnection
		init          sync.Once                       // Ensures a MuxManager can only be started once
		logger        log.Logger
		shutDown      channel.ShutdownOnce // when cancelled, the underlying transports will be stopped too
		closeChan     chan struct{}        // For Closable support
		wakeInterval  time.Duration
	}
	SessionWithConn struct {
		session *yamux.Session
		conn    net.Conn
	}
	MuxManager interface {
		IsClosed() bool
		Close()
		CloseChan() <-chan struct{}
		Connect(clientMetrics *grpcprom.ClientMetrics) (*grpc.ClientConn, error)
		Serve(server *grpc.Server) error
		Start() error
		ReplaceConnection(swc *SessionWithConn)
	}
)

func (s *SessionWithConn) IsClosed() bool {
	return s.session.IsClosed()
}

// NewMuxManager will wrap the provided logger with a tag identifying the logs, and handles initializing all the sync
// primitives
func NewMuxManager(cfg config.MuxTransportConfig, logger log.Logger) MuxManager {
	muxMgr := &muxManager{
		config:        cfg,
		muxConnection: atomic.Pointer[SessionWithConn]{},
		connAvailable: sync.Cond{L: &sync.Mutex{}},
		init:          sync.Once{},
		logger:        log.With(logger, tag.NewStringTag("component", "MuxManager")),
		shutDown:      channel.NewShutdownOnce(),
		closeChan:     make(chan struct{}),
		wakeInterval:  time.Second * 5,
	}
	return muxMgr
}

// Close closes the internal shutdown latch and sets the connection to nil. This will stop the connection provider:
// if you want to reopen connections you'll need to create a new MuxManager instance.
func (m *muxManager) Close() {
	m.shutDown.Shutdown()
	m.ReplaceConnection(nil)
	select {
	case <-m.closeChan:
	default:
		close(m.closeChan)
	}
}

// CloseChan is part of Closable
func (m *muxManager) CloseChan() <-chan struct{} {
	return m.closeChan
}

func (m *muxManager) IsClosed() bool {
	select {
	case <-m.closeChan:
		return true
	default:
		return false
	}
}

// Connect is part of the ClientTransport interface. This is used to establish an outbound grpc client.
// It needs to create a dialer, dial the remote host using the provided mux, and then return the new connection.
// This will block until a session is available.
func (m *muxManager) Connect(clientMetrics *grpcprom.ClientMetrics) (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return WithConnection(ctx, m, func(conn *SessionWithConn) (net.Conn, error) { return conn.session.Open() })
	}

	// Set hostname to unused since custom dialer is used.
	return grpcutil.Dial("unused", nil, clientMetrics, dialer)
}

// Serve is part of the ServerTransport interface. This is used to run a grpc server on the provided mux.
// This will block until a session is available.
func (m *muxManager) Serve(server *grpc.Server) error {
	_, err := WithConnection(context.Background(), m, func(s *SessionWithConn) (struct{}, error) {
		return struct{}{}, server.Serve(s.session)
	})
	return err
}

// WithConnection waits on connAvailable's condition until the pointer is non-null, then runs the provided function
// with that pointer.
// Warning: This function will panic if you try to pass it an alternative MuxManager struct
func WithConnection[T any](ctx context.Context, m MuxManager, f func(*SessionWithConn) (T, error)) (T, error) {
	if mm, ok := m.(*muxManager); ok {
		// Some reminders on condition variables: A condition variable is a multi-layered lock: First we lock this
		// outer lock, which protects an internal semaphore/waitgroup. That semaphore/waitgroup represents a queue of waiting threads.
		mm.connAvailable.L.Lock()
		for {
			if ctx.Err() != nil {
				var empty T
				return empty, ctx.Err()
			}
			if mm.shutDown.IsShutdown() {
				mm.connAvailable.L.Unlock()
				var empty T
				return empty, errors.New("the mux manager is shutting down")
			}
			// When we wait here, we add ourselves to the list of threads that should be restarted, let go of connAvailable.L,
			// and then suspend indefinitely. We will only be woken by Broadcast (wakes up every thread) or Signal (wakes up one thread)
			// As part of waking up from connAvailable.Wait, this thread will re-take connAvailable.L
			mm.connAvailable.Wait()
			if ptr := mm.muxConnection.Load(); ptr != nil && !ptr.IsClosed() {
				// Don't keep lock held while running f so that other code can use the connection
				mm.connAvailable.L.Unlock()
				ret, err := f(ptr)
				if err != nil {
					var empty T
					return empty, fmt.Errorf("the provided function threw error %w", err)
				}
				return ret, nil
			}
		}
	} else {
		panic("invalid mux manager type " + reflect.TypeOf(m).String())
	}

}

// TryConnectionOrElse grabs whatever connection is available and runs f on that connection.
// The received SessionWithConn is guaranteed to be nil, a valid yamux session, or a closed yamux session
// Warning: This function will panic if you try to pass it an alternative MuxManager struct
func TryConnectionOrElse[T any](m MuxManager, f func(*SessionWithConn) T, other T) T {
	if mm, ok := m.(*muxManager); ok {
		conn := mm.muxConnection.Load()
		if conn == nil {
			return other
		}
		return f(conn)
	} else {
		panic("Unsupported MuxManager type " + reflect.TypeOf(m).String())
	}
}

// ReplaceConnection sets the new SessionWithConn object, and notifies all the waiting threads that there's new data
// ReplaceConnection holds connAvailable.L to ensure there's no race conditions while threads prepare to wait.
func (m *muxManager) ReplaceConnection(swc *SessionWithConn) {
	// Waiting on connAvailable.L here ensures no other threads are in the process of figuring out they should be notified
	// when Broadcast runs.
	m.connAvailable.L.Lock()
	defer m.connAvailable.L.Unlock()
	// Make sure the existing conn is fully closed
	existingConn := m.muxConnection.Load()
	if existingConn != nil {
		_ = existingConn.session.Close()
		_ = existingConn.conn.Close()
	}
	// Now add new conn
	m.muxConnection.Store(swc)
	// Now notify
	m.connAvailable.Broadcast()
}

// Start looks at the config, constructs the appropriate MuxProvider and Starts it. Once started, the provider will
// run until shutDown is closed. If the provider panics for some reason, it will close shutDown itself and terminate.
func (m *muxManager) Start() error {
	var err error
	m.init.Do(func() {
		// Wake up the threads waiting on connections every 5 seconds so that we can obey any context timeouts.
		go func() {
			wakeThreads := time.NewTicker(m.wakeInterval)
			for !m.shutDown.IsShutdown() {
				select {
				case <-wakeThreads.C:
					m.connAvailable.Broadcast()
				case <-m.closeChan:
					return
				}
			}
		}()
		switch m.config.Mode {
		case config.ClientMode:
			m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Client))
			metricLabels := []string{m.config.Client.ServerAddress,
				string(m.config.Mode),
				m.config.Name,
			}
			var provider MuxProvider
			provider, err = NewMuxEstablisherProvider(m.config.Name, m.ReplaceConnection, m.config.Client, metricLabels, m.logger, m.shutDown)
			if err != nil {
				return
			}
			provider.Start()
		case config.ServerMode:
			m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Server))
			metricLabels := []string{m.config.Server.ListenAddress,
				string(m.config.Mode),
				m.config.Name,
			}
			var provider MuxProvider
			provider, err = NewMuxReceiverProvider(m.config.Name, m.ReplaceConnection, m.config.Server, metricLabels, m.logger)
			if err != nil {
				return
			}
			provider.Start()
		default:
			err = fmt.Errorf("invalid multiplexed transport mode: name %s, mode %s", m.config.Name, m.config.Mode)
		}
	})
	return err
}
