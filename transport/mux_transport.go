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
		cleanups []func()
		isReady  bool
	}
)

func (m *muxTransport) Connect() (*grpc.ClientConn, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return m.session.Open()
	}

	// Set hostname to unused since custom dialer is used.
	return dial("unused", nil, dialer)
}

func (m *muxTransport) Serve(server *grpc.Server) error {
	return server.Serve(m.session)
}

// addCleanup method registers functions to clean up underlying connection as
// yamux.Session.Close doesn't addCleanup connection.
// Release functions will be called in last added first called order.
// It is not thread-safe.
func (m *muxTransport) addCleanup(f func()) {
	m.cleanups = append(m.cleanups, f)
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

	m.addCleanup(func() { _ = conn.Close() })

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

	m.addCleanup(func() { _ = listener.Close() })

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

	m.addCleanup(func() { _ = conn.Close() })
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

		for len(m.cleanups) > 0 {
			last := len(m.cleanups) - 1
			m.cleanups[last]()
			m.cleanups = m.cleanups[:last]
		}

		m.isReady = false
	}
}
