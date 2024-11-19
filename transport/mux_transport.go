package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

const (
	statusInitialized int32 = 0
	statusStarted     int32 = 1
	statusStopped     int32 = 2
)

type (
	muxTransportImpl struct {
		session *yamux.Session
		conn    net.Conn
		closeCh chan struct{}
	}

	muxConnectMananger struct {
		config       config.MuxTransportConfig
		muxTransport *muxTransportImpl
		shutdownCh   chan struct{}
		connectedCh  chan struct{}
		mu           sync.Mutex
		wg           sync.WaitGroup
		logger       log.Logger
		status       int32
	}
)

func newMuxTransport(conn net.Conn, session *yamux.Session) *muxTransportImpl {
	return &muxTransportImpl{
		conn:    conn,
		session: session,
		closeCh: make(chan struct{}),
	}
}

func (s *muxTransportImpl) Connect() (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return s.session.Open()
	}

	// Set hostname to unused since custom dialer is used.
	return dial("unused", nil, dialer)
}

func (s *muxTransportImpl) Serve(server *grpc.Server) error {
	return server.Serve(s.session)
}

func (m *muxTransportImpl) CloseChan() <-chan struct{} {
	return m.closeCh
}

func (s *muxTransportImpl) closeSession() {
	s.conn.Close()
	s.session.Close()
}

func (s *muxTransportImpl) Close() {
	s.closeSession()
	// Wait for connection manager to notify close is completed.
	<-s.closeCh
}

func newMuxConnectManager(cfg config.MuxTransportConfig, logger log.Logger) *muxConnectMananger {
	return &muxConnectMananger{
		config: cfg,
		logger: log.With(logger, tag.NewStringTag("Name", cfg.Name), tag.NewStringTag("Mode", string(cfg.Mode))),
		status: statusInitialized,
	}
}

func (m *muxConnectMananger) open() (MuxTransport, error) {
	if !m.isStarted() {
		return nil, fmt.Errorf("Connection manager is not running.")
	}

	// Wait for transport to be connected
	m.mu.Lock()
	<-m.connectedCh
	muxTransport := m.muxTransport
	m.mu.Unlock()

	if muxTransport.session.IsClosed() {
		return nil, fmt.Errorf("Session is closed")
	}
	return muxTransport, nil
}

func (m *muxConnectMananger) isShuttingDown() bool {
	select {
	case <-m.shutdownCh:
		return true
	default:
		return false
	}
}

func (m *muxConnectMananger) serverLoop(setting config.TCPServerSetting) error {
	listener, err := net.Listen("tcp", setting.ListenAddress)
	if err != nil {
		return err
	}

	m.wg.Add(1)
	go func() {
		defer func() {
			listener.Close()
			m.wg.Done()
		}()

		for {
			select {
			case <-m.shutdownCh:
				return
			default:
				m.logger.Debug("listener accept new connection")
				// Accept a TCP connection
				server, err := listener.Accept()
				if err != nil {
					if m.isShuttingDown() {
						return
					}

					m.logger.Fatal("listener.Accept failed", tag.Error(err))
				}

				var conn net.Conn
				if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
					tlsConfig, err := encryption.GetServerTLSConfig(tlsCfg)
					if err != nil {
						m.logger.Fatal("GetClientTLSConfig failed", tag.Error(err))
					}

					conn = tls.Server(server, tlsConfig)
				} else {
					conn = server
				}

				session, err := yamux.Server(conn, nil)
				if err != nil {
					m.logger.Fatal("yamux.Server failed", tag.Error(err))
				}

				m.muxTransport = newMuxTransport(conn, session)
				m.waitForReconnect()
			}
		}
	}()

	go func() {
		// wait for shutdown
		<-m.shutdownCh
		listener.Close()
	}()

	return nil
}

func (m *muxConnectMananger) clientLoop(setting config.TCPClientSetting) error {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
	Loop:
		for {
			select {
			case <-m.shutdownCh:
				return
			default:
				m.logger.Debug("Dail up")

				client, err := net.DialTimeout("tcp", setting.ServerAddress, 5*time.Second)
				if err != nil {
					m.logger.Error("failed to dail up", tag.Error(err))
					continue Loop
				}

				var conn net.Conn
				if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
					tlsConfig, err := encryption.GetClientTLSConfig(tlsCfg)
					if err != nil {
						m.logger.Fatal("GetClientTLSConfig failed")
					}

					conn = tls.Client(client, tlsConfig)
				} else {
					conn = client
				}

				session, err := yamux.Client(conn, nil)
				if err != nil {
					m.logger.Fatal("yamux.Client failed")
				}

				m.muxTransport = newMuxTransport(conn, session)
				m.waitForReconnect()
			}
		}
	}()

	return nil
}

func (m *muxConnectMananger) start() error {
	if !atomic.CompareAndSwapInt32(
		&m.status,
		statusInitialized,
		statusStarted,
	) {
		return fmt.Errorf("Connetion manager can't be started. status: %d", m.getStatus())
	}

	m.shutdownCh = make(chan struct{})
	m.connectedCh = make(chan struct{})

	switch m.config.Mode {
	case config.ClientMode:
		m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Client))
		if err := m.clientLoop(m.config.Client); err != nil {
			return err
		}
	case config.ServerMode:
		m.logger.Info(fmt.Sprintf("Start ConnectMananger with Config: %v", m.config.Server))
		if err := m.serverLoop(m.config.Server); err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid multiplexed transport mode: name %s, mode %s.", m.config.Name, m.config.Mode)
	}

	return nil
}

func (m *muxConnectMananger) isStarted() bool {
	return atomic.LoadInt32(&m.status) == statusStarted
}

func (m *muxConnectMananger) getStatus() int32 {
	return atomic.LoadInt32(&m.status)
}

func (m *muxConnectMananger) stop() {
	if !atomic.CompareAndSwapInt32(
		&m.status,
		statusStarted,
		statusStopped,
	) {
		return
	}

	close(m.shutdownCh)
	m.wg.Wait()
	m.logger.Info("Connection manager stopped")
}

func (m *muxConnectMananger) waitForReconnect() {
	// Notify transport is connected
	close(m.connectedCh)

	m.logger.Debug("connected")

	select {
	case <-m.shutdownCh:
		m.muxTransport.closeSession()
	case <-m.muxTransport.session.CloseChan():
		m.muxTransport.closeSession()
	}

	m.logger.Debug("disconnected")

	// Initialize connectedCh and muxTransport
	m.mu.Lock()
	m.connectedCh = make(chan struct{})
	muxTransport := m.muxTransport
	m.muxTransport = nil
	m.mu.Unlock()

	// Notify transport is closed
	close(muxTransport.closeCh)
}
