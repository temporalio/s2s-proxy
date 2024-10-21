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
	ClientTransport interface {
		Connect() (*grpc.ClientConn, error)
	}

	ServerTransport interface {
		Serve(server *grpc.Server) error
	}

	TransportProvider interface {
		CreateClientTransport(cfg config.ClientConfig) (ClientTransport, error)
		CreateServerTransport(cfg config.ServerConfig) (ServerTransport, error)
	}

	multiplexTransport struct {
		mode    config.MultiplexMode
		session *yamux.Session
		isReady bool
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

	transportProviderImpl struct {
		multiplexTransports map[string]*multiplexTransport
	}
)

func createClientSession(setting config.TCPClientSetting) (*yamux.Session, error) {
	conn, err := net.DialTimeout("tcp", setting.ServerAddress, 10*time.Second)
	if err != nil {
		return nil, err
	}

	return yamux.Client(conn, nil)
}

func createServerSession(setting config.TCPServerSetting) (*yamux.Session, error) {
	listener, err := net.Listen("tcp", setting.ListenAddress)
	if err != nil {
		return nil, err
	}

	// Accept a TCP connection
	conn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	return yamux.Server(conn, nil)
}

func NewTransprotProvider(
	configProvider config.ConfigProvider,
) (TransportProvider, error) {
	provider := &transportProviderImpl{
		multiplexTransports: make(map[string]*multiplexTransport),
	}

	s2sConfig := configProvider.GetS2SProxyConfig()
	for _, multiplex := range s2sConfig.MultiplexTransports {
		var session *yamux.Session
		var err error

		switch multiplex.Mode {
		case config.ClientMode:
			if multiplex.Client == nil {
				return nil, fmt.Errorf("invalid multiplexed transport for %s: client setting is not provided.", multiplex.Name)
			}

			session, err = createClientSession(*multiplex.Client)
			if err != nil {
				return nil, err
			}

		case config.ServerMode:
			if multiplex.Server == nil {
				return nil, fmt.Errorf("invalid multiplexed transport for %s: server setting is not provided.", multiplex.Name)
			}

			session, err = createServerSession(*multiplex.Server)
			if err != nil {
				return nil, err
			}

		default:
			return nil, fmt.Errorf("invalid multiplexed transport for %s: unknown mode %s.", multiplex.Name, multiplex.Mode)
		}

		provider.multiplexTransports[multiplex.Name] = &multiplexTransport{
			mode:    multiplex.Mode,
			session: session,
			isReady: true,
		}
	}

	return provider, nil
}

func (t *transportProviderImpl) getMultiplexSession(name string) (*yamux.Session, error) {
	ts := t.multiplexTransports[name]
	if ts == nil {
		return nil, fmt.Errorf("could not find transport  %s", name)
	}

	if !ts.isReady {
		return nil, fmt.Errorf("multiplex session for transport %s is not ready", name)
	}

	return ts.session, nil
}

func (t *transportProviderImpl) CreateServerTransport(cfg config.ServerConfig) (ServerTransport, error) {
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

func (t *transportProviderImpl) CreateClientTransport(cfg config.ClientConfig) (ClientTransport, error) {
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

func NewSessionClient() ClientTransport {
	return &sessionTransport{}
}

func NewSessionServer() ServerTransport {
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
