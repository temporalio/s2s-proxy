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
	muxTransport struct {
		config      config.MuxTransportConfig
		session     *yamux.Session
		conn        net.Conn
		isConnected bool
	}
)

func newMuxTransport(cfg config.MuxTransportConfig) *muxTransport {
	return &muxTransport{
		config: cfg,
	}
}

func (m *muxTransport) Connect() (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		if !m.isConnected {
			return nil, fmt.Errorf("Transport Connect failed: mux transport is not connected")
		}

		return m.session.Open()
	}

	// Set hostname to unused since custom dialer is used.
	return dial("unused", nil, dialer)
}

func (m *muxTransport) Serve(server *grpc.Server) error {
	if !m.isConnected {
		return fmt.Errorf("Transport Serve failed: mux transport is not connected")
	}

	return server.Serve(m.session)
}

func (m *muxTransport) createClientConn() error {
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

func (m *muxTransport) createServerConn() error {
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

func (m *muxTransport) establishConnection() error {
	switch m.config.Mode {
	case config.ClientMode:
		return m.createClientConn()
	case config.ServerMode:
		return m.createServerConn()
	default:
		return fmt.Errorf("invalid mux transport mode: name %s, mode %s.", m.config.Name, m.config.Mode)
	}
}

func (m *muxTransport) waitForClose() {
	defer func() {
		m.conn.Close()
		m.session.Close()
		m.isConnected = false
	}()

	// wait for session to close
	<-m.session.CloseChan()
}

func (m *muxTransport) start() error {
	// Create initial connection
	if err := m.establishConnection(); err != nil {
		return err
	}

	go m.waitForClose()
	return nil
}

func (m *muxTransport) Close() {
	m.session.Close()
}

func (m *muxTransport) IsClosed() bool {
	return !m.isConnected
}

func (m *muxTransport) CloseChan() <-chan struct{} {
	return m.session.CloseChan()
}
