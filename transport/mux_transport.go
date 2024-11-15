package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"google.golang.org/grpc"
)

type (
	muxTransportImpl struct {
		config      config.MuxTransportConfig
		session     *yamux.Session
		conn        net.Conn
		shutDownCh  chan struct{}
		isConnected bool
	}
)

func newMuxTransport(cfg config.MuxTransportConfig) *muxTransportImpl {
	return &muxTransportImpl{
		config:     cfg,
		shutDownCh: make(chan struct{}),
	}
}

func (m *muxTransportImpl) Connect() (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		if !m.isConnected {
			return nil, fmt.Errorf("Transport Connect failed: mux transport is not connected")
		}

		return m.session.Open()
	}

	// Set hostname to unused since custom dialer is used.
	return dial("unused", nil, dialer)
}

func (m *muxTransportImpl) Serve(server *grpc.Server) error {
	if !m.isConnected {
		return fmt.Errorf("Transport Serve failed: mux transport is not connected")
	}

	return server.Serve(m.session)
}

func (m *muxTransportImpl) createClientConn() error {
	setting := m.config.Client
	if setting == nil {
		return fmt.Errorf("invalid client mux transport setting: %v", m.config)
	}

	client, err := net.DialTimeout("tcp", setting.ServerAddress, 10*time.Second)
	if err != nil {
		return err
	}

	var conn net.Conn
	if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
		tlsConfig, err := encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			return err
		}

		conn = tls.Client(client, tlsConfig)
	} else {
		conn = client
	}

	m.conn = conn
	m.session, err = yamux.Client(conn, nil)
	if err != nil {
		return err
	}

	m.isConnected = true
	return nil
}

func (m *muxTransportImpl) createServerConn() error {
	setting := m.config.Server
	if setting == nil {
		return fmt.Errorf("invalid client mux transport setting: %v", m.config)
	}

	listener, err := net.Listen("tcp", setting.ListenAddress)
	if err != nil {
		return err
	}

	// Accept a TCP connection
	server, err := listener.Accept()
	if err != nil {
		return err
	}

	var conn net.Conn
	if tlsCfg := setting.TLS; tlsCfg.IsEnabled() {
		tlsConfig, err := encryption.GetServerTLSConfig(tlsCfg)
		if err != nil {
			return err
		}

		conn = tls.Server(server, tlsConfig)
	} else {
		conn = server
	}

	m.conn = conn
	m.session, err = yamux.Server(conn, nil)
	if err != nil {
		return err
	}

	m.isConnected = true
	return nil
}

func (m *muxTransportImpl) establishConnection() error {
	switch m.config.Mode {
	case config.ClientMode:
		return m.createClientConn()
	case config.ServerMode:
		return m.createServerConn()
	default:
		return fmt.Errorf("invalid mux transport mode: name %s, mode %s.", m.config.Name, m.config.Mode)
	}
}

func (m *muxTransportImpl) waitForClose() {
	defer func() {
		m.conn.Close()
		m.isConnected = false
		close(m.shutDownCh)
	}()

	// wait for session to close
	<-m.session.CloseChan()
}

func (m *muxTransportImpl) start() error {
	// Create initial connection
	if err := m.establishConnection(); err != nil {
		return err
	}

	go m.waitForClose()
	return nil
}

func (m *muxTransportImpl) Close() {
	m.session.Close()
	// wait for shutdown
	<-m.shutDownCh
}

func (m *muxTransportImpl) IsClosed() bool {
	return !m.isConnected
}

func (m *muxTransportImpl) CloseChan() <-chan struct{} {
	return m.session.CloseChan()
}
