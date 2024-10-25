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
		config   config.MuxTransportConfig
		session  *yamux.Session
		closeFns []func()
		isReady  bool
	}
)

func (m *muxTransport) Connect() (*grpc.ClientConn, error) {
	conn, err := m.session.Open()
	if err != nil {
		return nil, err
	}

	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return conn, nil // Return the original TCP connection
	}

	return dial("unused", nil, dialer)
}

func (m *muxTransport) Serve(server *grpc.Server) error {
	return server.Serve(m.session)
}

func (m *muxTransport) startClient() error {
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

	m.closeFns = append(m.closeFns, func() { _ = conn.Close() })

	m.session, err = yamux.Client(conn, nil)
	if err != nil {
		return err
	}

	m.isReady = true
	return nil
}

func (m *muxTransport) startServer() error {
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

	m.closeFns = append(m.closeFns, func() { _ = listener.Close() })

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

	m.closeFns = append(m.closeFns, func() { _ = conn.Close() })
	m.session, err = yamux.Server(conn, nil)
	if err != nil {
		return err
	}
	m.isReady = true
	return nil
}

func (m *muxTransport) start() error {
	if m.isReady {
		return nil
	}

	switch m.config.Mode {
	case config.ClientMode:
		if err := m.startClient(); err != nil {
			return err
		}
	case config.ServerMode:
		if err := m.startServer(); err != nil {
			return err
		}

	default:
		return fmt.Errorf("invalid mux transport mode: name %s, mode %s.", m.config.Name, m.config.Mode)
	}

	return nil
}

func (m *muxTransport) stop() {
	if m.isReady && m.session != nil {
		m.session.Close()

		for len(m.closeFns) > 0 {
			last := len(m.closeFns) - 1
			m.closeFns[last]()
			m.closeFns = m.closeFns[:last]
		}

		m.isReady = false
	}
}
