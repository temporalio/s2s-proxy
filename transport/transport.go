package transport

import (
	"fmt"
	"net"
	"time"

	"github.com/hashicorp/yamux"
	"github.com/temporalio/s2s-proxy/config"
	"google.golang.org/grpc"
)

type (
	ClientTransport interface {
		Connect() (*grpc.ClientConn, error)
	}

	ServerTransport interface {
		Serve(server *grpc.Server) error
	}

	TransportManager interface {
		Start() error
		Stop()
		CreateClientTransport(cfg config.ClientConfig) (ClientTransport, error)
		CreateServerTransport(cfg config.ServerConfig) (ServerTransport, error)
	}

	transportManagerImpl struct {
		muxTransports map[string]*muxTransport
		isReady       bool
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

func NewTransportManager(
	configProvider config.ConfigProvider,
) TransportManager {
	s2sConfig := configProvider.GetS2SProxyConfig()
	manager := &transportManagerImpl{
		muxTransports: make(map[string]*muxTransport),
	}

	for _, multiplex := range s2sConfig.MuxTransports {
		manager.muxTransports[multiplex.Name] = &muxTransport{
			config: multiplex,
		}
	}

	return manager
}

func (t *transportManagerImpl) Start() error {
	if t.isReady {
		return nil
	}

	for _, mux := range t.muxTransports {
		var err error

		cfg := mux.config
		switch cfg.Mode {
		case config.ClientMode:
			if cfg.Client == nil {
				return fmt.Errorf("invalid multiplexed transport for %s: client setting is not provided.", cfg.Name)
			}

			mux.session, err = createClientSession(*cfg.Client)
			if err != nil {
				return err
			}

		case config.ServerMode:
			if cfg.Server == nil {
				return fmt.Errorf("invalid multiplexed transport for %s: server setting is not provided.", cfg.Name)
			}

			mux.session, err = createServerSession(*cfg.Server)
			if err != nil {
				return err
			}

		default:
			return fmt.Errorf("invalid multiplexed transport for %s: unknown mode %s.", cfg.Name, cfg.Mode)
		}

		mux.isReady = true
	}

	t.isReady = true
	return nil
}

func (t *transportManagerImpl) Stop() {
	if !t.isReady {
		return
	}

	for _, mux := range t.muxTransports {
		if mux.isReady && mux.session != nil {
			mux.session.Close()
		}
	}
}

func (t *transportManagerImpl) getMuxTransport(name string) (*muxTransport, error) {
	ts := t.muxTransports[name]
	if ts == nil {
		return nil, fmt.Errorf("could not find transport  %s", name)
	}

	if !ts.isReady {
		return nil, fmt.Errorf("multiplex session for transport %s is not ready", name)
	}

	return ts, nil
}

func (t *transportManagerImpl) CreateServerTransport(cfg config.ServerConfig) (ServerTransport, error) {
	if cfg.Type == config.MuxTransport {
		return t.getMuxTransport(cfg.MuxTransportName)
	}

	// use TCP as default transport
	return &tcpServer{
		config: cfg.TCPServerSetting,
	}, nil
}

func (t *transportManagerImpl) CreateClientTransport(cfg config.ClientConfig) (ClientTransport, error) {
	if cfg.Type == config.MuxTransport {
		return t.getMuxTransport(cfg.MuxTransportName)
	}

	// use TCP as default transport
	return &tcpClient{
		config: cfg.TCPClientSetting,
	}, nil
}
