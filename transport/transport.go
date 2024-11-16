package transport

import (
	"fmt"

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
		CloseChan() <-chan struct{}
		Close()
	}

	TransportManager struct {
		connectManagers map[string]*MuxConnectMananger
		logger          log.Logger
	}
)

func NewTransportManager(
	configProvider config.ConfigProvider,
	logger log.Logger,
) *TransportManager {

	cm := make(map[string]*MuxConnectMananger)
	s2sConfig := configProvider.GetS2SProxyConfig()
	for _, cfg := range s2sConfig.MuxTransports {
		cm[cfg.Name] = newMuxConnectManager(cfg, logger)
	}

	return &TransportManager{
		connectManagers: cm,
		logger:          logger,
	}
}

func (tm *TransportManager) GetConnectManager(name string) (*MuxConnectMananger, error) {
	mux := tm.connectManagers[name]
	if mux == nil {
		return nil, fmt.Errorf("Multiplexed transport %s is not found", name)
	}

	return mux, nil
}

func (tm *TransportManager) Start() error {
	for _, cm := range tm.connectManagers {
		if err := cm.start(); err != nil {
			return err
		}
	}

	return nil
}

func (tm *TransportManager) Stop() {
	for _, cm := range tm.connectManagers {
		cm.stop()
	}
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
