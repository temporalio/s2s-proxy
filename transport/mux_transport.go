package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	muxTransportImpl struct {
		session *yamux.Session
		conn    net.Conn
		closeCh chan struct{}
	}

	MuxConnectMananger struct {
		config       config.MuxTransportConfig
		muxTransport *muxTransportImpl
		shutdownCh   chan struct{}
		connectedCh  chan struct{}
		wg           sync.WaitGroup
		logger       log.Logger
		started      bool
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

func (s *muxTransportImpl) Close() {
	s.conn.Close()
	s.session.Close()

	// Wait for connection manager to notify close is completed.
	<-s.closeCh
}

func newMuxConnectManager(cfg config.MuxTransportConfig, logger log.Logger) *MuxConnectMananger {
	return &MuxConnectMananger{
		config:      cfg,
		logger:      log.With(logger, tag.NewStringTag("Name", cfg.Name), tag.NewStringTag("Mode", string(cfg.Mode))),
		shutdownCh:  make(chan struct{}),
		connectedCh: make(chan struct{}),
	}
}

func (m *MuxConnectMananger) Open() (MuxTransport, error) {
	if !m.started {
		return nil, fmt.Errorf("Connection manager is stopped")
	}

	// Wait for transport to be connected
	<-m.connectedCh

	if m.muxTransport.session.IsClosed() {
		// this should not happen
		return nil, fmt.Errorf("Session is closed")
	}
	return m.muxTransport, nil
}

func (m *MuxConnectMananger) isShuttingDown() bool {
	select {
	case <-m.shutdownCh:
		return true
	default:
		return false
	}
}

func (m *MuxConnectMananger) serverLoop(setting *config.TCPServerSetting) error {
	if setting == nil {
		return fmt.Errorf("invalid server mux transport setting: %v", m.config)
	}

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
				m.logger.Info("listener accept new connection")
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

func (m *MuxConnectMananger) clientLoop(setting *config.TCPClientSetting) error {
	if setting == nil {
		return fmt.Errorf("invalid client mux transport setting: %v", m.config)
	}

	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
	Loop:
		for {
			select {
			case <-m.shutdownCh:
				return
			default:
				m.logger.Info("try to dail up")
				client, err := net.DialTimeout("tcp", setting.ServerAddress, 5*time.Second)
				if err != nil {
					m.logger.Error("failed to dail up")
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

func (m *MuxConnectMananger) start() error {
	m.logger.Info("start connection manager")
	switch m.config.Mode {
	case config.ClientMode:
		if err := m.clientLoop(m.config.Client); err != nil {
			return err
		}
	case config.ServerMode:
		if err := m.serverLoop(m.config.Server); err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid multiplexed transport mode: name %s, mode %s.", m.config.Name, m.config.Mode)
	}

	m.started = true
	return nil
}

func (m *MuxConnectMananger) stop() {
	close(m.shutdownCh)
	m.wg.Wait()
	m.started = false
	m.logger.Info("Connection manager stopped")
}

func (m *MuxConnectMananger) waitForReconnect() {
	// Notify transport is connected
	close(m.connectedCh)

	m.logger.Info("connected")

	select {
	case <-m.shutdownCh:
	case <-m.muxTransport.session.CloseChan():
		m.logger.Info("session closed")
	}

	// create a new connected channel for reconnect
	m.connectedCh = make(chan struct{})

	// Notify transport is closed
	close(m.muxTransport.closeCh)
	m.muxTransport = nil
}
