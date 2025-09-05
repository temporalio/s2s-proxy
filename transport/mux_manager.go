package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

type (
	muxManager struct {
		config        config.MuxTransportConfig
		muxConnection atomic.Pointer[SessionWithConn] // Underlying mux value. This starts as nil, and is set by the provider.
		connAvailable sync.Cond                       // Condition lock for muxConnection. Used to notify threads waiting in WithConnection
		init          sync.Once                       // Ensures a muxManager can only be started once
		logger        log.Logger
		ctx           context.Context         // when cancelled, the underlying transports will be stopped too
		reportFatal   context.CancelCauseFunc // Used by the mux provider to report that it has died
	}
	SessionWithConn struct {
		session *yamux.Session
		conn    net.Conn
	}
)

func (s *SessionWithConn) IsClosed() bool {
	return s.session.IsClosed()
}

// nolint:unused
func newMuxManager(cfg config.MuxTransportConfig, logger log.Logger, ctx context.Context) (*muxManager, context.CancelCauseFunc) {
	muxCtx, cancel := context.WithCancelCause(ctx)
	muxMgr := &muxManager{
		config:        cfg,
		muxConnection: atomic.Pointer[SessionWithConn]{},
		connAvailable: sync.Cond{L: &sync.Mutex{}},
		init:          sync.Once{},
		logger:        log.With(logger, tag.NewStringTag("component", "muxManager")),
		ctx:           muxCtx,
	}
	// Wrap the typical cancel function with one that ensures all blocked threads are freed
	muxMgr.reportFatal = func(cause error) {
		// We need context.cancel to "happen-before" all the threads on this condition are released. So we hold the lock
		muxMgr.connAvailable.L.Lock()
		cancel(cause)
		muxMgr.connAvailable.Broadcast()
		muxMgr.connAvailable.L.Unlock()
	}
	return muxMgr, muxMgr.reportFatal
}

// WithConnection waits on connAvailable's condition until the pointer is non-null, then runs the provided function
// with that pointer.
func (m *muxManager) WithConnection(f func(*SessionWithConn) error) error {
	m.connAvailable.L.Lock()
	for {
		if m.ctx.Err() != nil {
			m.connAvailable.L.Unlock()
			return errors.New("the mux manager is shutting down")
		}
		m.connAvailable.Wait()
		if ptr := m.muxConnection.Load(); ptr != nil && !ptr.IsClosed() {
			// Don't keep lock held while running f so that other code can use the connection
			m.connAvailable.L.Unlock()
			err := f(ptr)
			if err != nil {
				return fmt.Errorf("the provided function threw error %w", err)
			}
			return nil
		}
	}
}

// TryConnectionOrElse grabs whatever connection is available and runs f on that connection.
// The received SessionWithConn is guaranteed to be nil, a valid yamux session, or a closed yamux session
func TryConnectionOrElse[T any](m *muxManager, f func(*SessionWithConn) T, other T) T {
	conn := m.muxConnection.Load()
	if conn == nil {
		return other
	}
	return f(conn)
}

func (m *muxManager) ReplaceConnection(session *yamux.Session, conn net.Conn) {
	m.connAvailable.L.Lock()
	defer m.connAvailable.L.Unlock()
	// Make sure the existing conn is fully closed
	existingConn := m.muxConnection.Load()
	_ = existingConn.session.Close()
	_ = existingConn.conn.Close()
	// Now add new conn
	m.muxConnection.Store(&SessionWithConn{session: session, conn: conn})
	// Now notify
	m.connAvailable.Broadcast()
}

// Start looks at the config, constructs the appropriate MuxProvider and Starts it. Once started, the provider will
// run until the context provided to this muxManager is cancelled. If the provider panics for some reason, it will
// call reportFatal, which should cancel the muxManager's context.
func (m *muxManager) Start() error {
	var err error
	m.init.Do(func() {
		switch m.config.Mode {
		case config.ClientMode:
			m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Client))
			metricLabels := []string{m.config.Client.ServerAddress,
				string(m.config.Mode),
				m.config.Name,
			}
			var provider *mux.MuxProvider
			provider, err = mux.NewMuxEstablisherProvider(m.config.Name, m.ReplaceConnection, m.config.Client, metricLabels, m.logger, m.ctx, m.reportFatal)
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
			var provider *mux.MuxProvider
			provider, err = mux.NewMuxReceiverProvider(m.config.Name, m.ReplaceConnection, m.config.Server, metricLabels, m.logger, m.ctx, m.reportFatal)
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
