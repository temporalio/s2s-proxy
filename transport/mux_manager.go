package transport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport/muxproviders"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

type (
	muxManager struct {
		config        config.MuxTransportConfig
		muxConnection atomic.Pointer[SessionWithConn]
		connAvailable sync.Cond
		init          sync.Once
		logger        log.Logger
		ctx           context.Context
	}
	SessionWithConn struct {
		session *yamux.Session
		conn    net.Conn
	}
)

func (s *SessionWithConn) IsClosed() bool {
	return s.IsClosed()
}

func newMuxManager(cfg config.MuxTransportConfig, logger log.Logger, ctx context.Context) *muxManager {
	return &muxManager{
		config:        cfg,
		muxConnection: atomic.Pointer[SessionWithConn]{},
		connAvailable: sync.Cond{L: &sync.Mutex{}},
		init:          sync.Once{},
		logger:        log.With(logger, tag.NewStringTag("component", "muxManager")),
		ctx:           ctx,
	}
}

func (m *muxManager) IsShuttingDown() bool {
	select {
	case <-m.ctx.Done():
		return true
	default:
		return false
	}
}

// WithConnection waits on connAvailable's condition until the pointer is non-null, then runs the provided function
// with that pointer.
func (m *muxManager) WithConnection(f func(*SessionWithConn) error) error {
	m.connAvailable.L.Lock()
	for {
		if m.IsShuttingDown() {
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
	m.muxConnection.Store(&SessionWithConn{session: session, conn: conn})
}

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
			var provider *muxproviders.MuxProvider
			provider, err = muxproviders.NewMuxEstablisherProvider(m.config.Name, m.ReplaceConnection, m.config.Client, metricLabels, m.logger, m.ctx)
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
			var provider *muxproviders.MuxProvider
			provider, err = muxproviders.NewMuxReceiverProvider(m.config.Name, m.ReplaceConnection, m.config.Server, metricLabels, m.logger, m.ctx)
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
