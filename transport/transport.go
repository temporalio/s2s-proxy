package transport

import (
	"fmt"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
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
		CreateClientTransport(cfg config.ProxyClientConfig) (ClientTransport, error)
		CreateServerTransport(cfg config.ProxyServerConfig) (ServerTransport, error)
	}

	transportManagerImpl struct {
		muxTransports map[string]*muxTransport
		isReady       bool
		logger        log.Logger
	}
)

func NewTransportManager(
	configProvider config.ConfigProvider,
	logger log.Logger,
) TransportManager {
	s2sConfig := configProvider.GetS2SProxyConfig()
	muxTransports := make(map[string]*muxTransport)
	for _, cfg := range s2sConfig.MuxTransports {
		muxTransports[cfg.Name] = &muxTransport{
			config: cfg,
		}
	}

	return &transportManagerImpl{
		muxTransports: muxTransports,
		logger:        logger,
	}
}

func (t *transportManagerImpl) Start() error {
	if t.isReady {
		return nil
	}

	t.logger.Info("Starting transport manager")
	for _, mux := range t.muxTransports {
		t.logger.Info("Starting mux transport", tag.NewAnyTag("Mode", mux.config.Mode), tag.Name(mux.config.Name))

		if err := mux.start(); err != nil {
			return err
		}
	}

	t.isReady = true
	return nil
}

func (t *transportManagerImpl) Stop() {
	t.logger.Info("Stopping transport manager")

	if !t.isReady {
		return
	}

	for _, mux := range t.muxTransports {
		t.logger.Info("Stopping mux transport", tag.NewAnyTag("Mode", mux.config.Mode), tag.Name(mux.config.Name))
		mux.stop()
	}

	t.isReady = false
}

func (t *transportManagerImpl) getMuxTransport(name string) (*muxTransport, error) {
	ts := t.muxTransports[name]
	if ts == nil {
		return nil, fmt.Errorf("could not find transport  %s", name)
	}

	if !ts.isReady {
		return nil, fmt.Errorf("mutiplex transport %s is not ready", name)
	}

	return ts, nil
}

func (t *transportManagerImpl) CreateServerTransport(cfg config.ProxyServerConfig) (ServerTransport, error) {
	if cfg.Type == config.MuxTransport {
		return t.getMuxTransport(cfg.MuxTransportName)
	}

	// use TCP as default transport
	return &tcpServer{
		config: cfg.TCPServerSetting,
	}, nil
}

func (t *transportManagerImpl) CreateClientTransport(cfg config.ProxyClientConfig) (ClientTransport, error) {
	if cfg.Type == config.MuxTransport {
		return t.getMuxTransport(cfg.MuxTransportName)
	}

	// use TCP as default transport
	return &tcpClient{
		config: cfg.TCPClientSetting,
	}, nil
}
