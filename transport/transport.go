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
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type (
	Client interface {
		Connect() (*grpc.ClientConn, error)
	}

	Server interface {
		Serve(server *grpc.Server) error
	}
	streamClient struct {
		config  config.TCPClientSetting
		session *yamux.Session
	}

	streamServer struct {
		config  config.TCPServerSetting
		session *yamux.Session
	}

	tcpClient struct {
		config config.TCPClientSetting
		logger log.Logger
	}

	tcpServer struct {
		config config.TCPServerSetting
	}

	sessionTransport struct {
		session *yamux.Session
	}

	TransportProvider struct {
		transportConfig config.MultiplexTransportConfig
		streamClients   map[string]*streamClient
		streamServers   map[string]*streamServer
	}
)

func (c *streamClient) connect() error {
	conn, err := net.DialTimeout("tcp", c.config.ServerAddress, 10*time.Second)
	if err != nil {
		return err
	}
	c.session, err = yamux.Client(conn, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *streamClient) stop() {
}

func (s *streamServer) start() error {
	listener, err := net.Listen("tcp", s.config.ListenAddress)
	if err != nil {
		return err
	}

	// Accept a TCP connection
	conn, err := listener.Accept()
	if err != nil {
		return err
	}

	// Setup server side of yamux
	s.session, err = yamux.Server(conn, nil)
	if err != nil {
		return err
	}

	return nil
}

func (c *streamServer) stop() {
}

func NewTranprotProvider(cfg config.MultiplexTransportConfig) *TransportProvider {
	return &TransportProvider{
		transportConfig: cfg,
		streamClients:   make(map[string]*streamClient),
		streamServers:   make(map[string]*streamServer),
	}
}

func (t *TransportProvider) Init() error {
	for _, cfg := range t.transportConfig.Clients {
		t.streamClients[cfg.Name] = &streamClient{
			config: cfg.TCPClientSetting,
		}
	}

	for _, cfg := range t.transportConfig.Servers {
		t.streamServers[cfg.Name] = &streamServer{
			config: cfg.TCPServerSetting,
		}
	}

	for _, server := range t.streamServers {
		if err := server.start(); err != nil {
			return err
		}
	}

	for _, client := range t.streamClients {
		if err := client.connect(); err != nil {
			return err
		}
	}

	return nil
}

func (t *TransportProvider) getMultiplexSession(name string) (*yamux.Session, error) {
	client := t.streamClients[name]
	if client != nil {
		return client.session, nil
	}

	server := t.streamServers[name]
	if server == nil {
		return nil, fmt.Errorf("not able to find session")
	}

	return server.session, nil
}

func (t *TransportProvider) CreateServerTransport(cfg config.ServerConfig) (Server, error) {
	if cfg.Type == config.MultiplexTransport {
		session, err := t.getMultiplexSession(cfg.MultiplexerName)
		if err != nil {
			return nil, err
		}
		return &sessionTransport{
			session: session,
		}, nil
	}

	// use TCP as default transport
	return &tcpServer{
		config: cfg.TCPServerSetting,
	}, nil
}

func (t *TransportProvider) CreateClientTransport(cfg config.ClientConfig) (Client, error) {
	if cfg.Type == config.MultiplexTransport {
		session, err := t.getMultiplexSession(cfg.MultiplexerName)
		if err != nil {
			return nil, err
		}
		return &sessionTransport{
			session: session,
		}, nil
	}

	// use TCP as default transport
	return &tcpClient{
		config: cfg.TCPClientSetting,
	}, nil
}

func (c *tcpClient) Connect() (*grpc.ClientConn, error) {
	var tlsConfig *tls.Config
	var err error
	if tls := c.config.TLS; tls.IsEnabled() {
		tlsConfig, err = encryption.GetClientTLSConfig(tls)
		if err != nil {
			return nil, err
		}
	}

	return dial(c.config.ServerAddress, tlsConfig, c.logger)
}

func (s *tcpServer) Serve(server *grpc.Server) error {
	listener, err := net.Listen("tcp", s.config.ListenAddress)
	if err != nil {
		return err
	}

	return server.Serve(listener)
}

func NewSessionClient() Client {
	return &sessionTransport{}
}

func NewSessionServer() Server {
	return &sessionTransport{}
}

func (s *sessionTransport) Connect() (*grpc.ClientConn, error) {
	conn, err := s.session.Open()
	if err != nil {
		return nil, err
	}

	// Create a gRPC dialer using the existing connection
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		return conn, nil // Return the original TCP connection
	}

	// Establish a gRPC client connection using the custom dialer
	return grpc.Dial(
		"unused", // Address is ignored since we're using a custom dialer
		grpc.WithTransportCredentials(insecure.NewCredentials()), // No TLS for simplicity
		grpc.WithContextDialer(dialer),
	)
}

func (s *sessionTransport) Serve(server *grpc.Server) error {
	return server.Serve(s.session)
}
