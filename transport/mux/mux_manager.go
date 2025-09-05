package mux

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	MuxManager struct {
		config        config.MuxTransportConfig
		muxConnection atomic.Pointer[SessionWithConn] // Underlying mux value. This starts as nil, and is set by the provider.
		connAvailable sync.Cond                       // Condition lock for muxConnection. Used to notify threads waiting in WithConnection
		init          sync.Once                       // Ensures a MuxManager can only be started once
		logger        log.Logger
		shutDown      channel.ShutdownOnce // when cancelled, the underlying transports will be stopped too
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
func newMuxManager(cfg config.MuxTransportConfig, logger log.Logger) *MuxManager {
	muxMgr := &MuxManager{
		config:        cfg,
		muxConnection: atomic.Pointer[SessionWithConn]{},
		connAvailable: sync.Cond{L: &sync.Mutex{}},
		init:          sync.Once{},
		logger:        log.With(logger, tag.NewStringTag("component", "MuxManager")),
		shutDown:      channel.NewShutdownOnce(),
	}
	return muxMgr
}

func (m *MuxManager) ShutDown() {
	m.shutDown.Shutdown()
	m.ReplaceConnection(nil)
}

// WithConnection waits on connAvailable's condition until the pointer is non-null, then runs the provided function
// with that pointer.
func (m *MuxManager) WithConnection(f func(*SessionWithConn) error) error {
	m.connAvailable.L.Lock()
	for {
		if m.shutDown.IsShutdown() {
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
func TryConnectionOrElse[T any](m *MuxManager, f func(*SessionWithConn) T, other T) T {
	conn := m.muxConnection.Load()
	if conn == nil {
		return other
	}
	return f(conn)
}

func (m *MuxManager) ReplaceConnection(swc *SessionWithConn) {
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
func (m *MuxManager) Start() error {
	var err error
	m.init.Do(func() {
		switch m.config.Mode {
		case config.ClientMode:
			m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Client))
			metricLabels := []string{m.config.Client.ServerAddress,
				string(m.config.Mode),
				m.config.Name,
			}
			var provider *MuxProvider
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
			var provider *MuxProvider
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
