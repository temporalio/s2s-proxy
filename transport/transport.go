package transport

import (
	"fmt"
	"sync"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
)

type (
	ClientTransport interface {
		Connect() (*grpc.ClientConn, error)
	}

	ServerTransport interface {
		Serve(server *grpc.Server) error
	}

	MuxTransport interface {
		ClientTransport
		ServerTransport
		IsClosed() bool
		CloseChan() <-chan struct{}
		Close()
	}

	TransportManager interface {
		Open(transportName string) (MuxTransport, error)
	}

	transportManagerImpl struct {
		config        config.S2SProxyConfig
		muxTransports map[string]*muxTransportImpl
		mu            sync.Mutex
		logger        log.Logger
	}
)

func NewTransportManager(
	configProvider config.ConfigProvider,
	logger log.Logger,
) TransportManager {
	return &transportManagerImpl{
		config:        configProvider.GetS2SProxyConfig(),
		muxTransports: make(map[string]*muxTransportImpl),
		logger:        logger,
	}
}

func (cm *transportManagerImpl) getConfig(transportName string) *config.MuxTransportConfig {
	for _, cfg := range cm.config.MuxTransports {
		if cfg.Name == transportName {
			return &cfg
		}
	}

	return nil
}

func (cm *transportManagerImpl) Open(transportName string) (MuxTransport, error) {
	cfg := cm.getConfig(transportName)
	if cfg == nil {
		return nil, fmt.Errorf("Transport %s is not found", transportName)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	mux := cm.muxTransports[cfg.Name]
	if mux != nil && !mux.IsClosed() {
		return mux, nil
	}

	// Create a new transport
	mux = newMuxTransport(*cfg)
	if err := mux.start(); err != nil {
		return nil, err
	}

	cm.muxTransports[cfg.Name] = mux
	return mux, nil
}

func NewTCPClientTransport(cfg config.TCPClientSetting) ClientTransport {
	return &tcpClient{
		config: cfg,
	}
}

func NewTCPServerTransport(cfg config.TCPServerSetting) ServerTransport {
	return &tcpServer{
		config: cfg,
	}
}
