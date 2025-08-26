package transport

import (
	"fmt"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	ClientTransport interface {
		Connect(clientMetrics *grpcprom.ClientMetrics) (*grpc.ClientConn, error)
	}

	ServerTransport interface {
		Serve(server *grpc.Server) error
	}

	Closable interface {
		CloseChan() <-chan struct{}
		IsClosed() bool
	}

	MuxTransport interface {
		ClientTransport
		ServerTransport
		Closable
	}

	TransportManager struct {
		muxConnManagers map[string]*muxConnectManager
		logger          log.Logger
	}
)

func NewTransportManager(
	configProvider config.ConfigProvider,
	logger log.Logger,
) *TransportManager {

	muxConnManagers := make(map[string]*muxConnectManager)
	s2sConfig := configProvider.GetS2SProxyConfig()
	for _, cfg := range s2sConfig.MuxTransports {
		muxConnManagers[cfg.Name] = newMuxConnectManager(cfg, logger)
	}

	return &TransportManager{
		muxConnManagers: muxConnManagers,
		logger:          logger,
	}
}

func (tm *TransportManager) openMuxTransport(transportName string) (MuxTransport, error) {
	mux := tm.muxConnManagers[transportName]
	if mux == nil {
		return nil, fmt.Errorf("multiplexed transport %s is not found", transportName)
	}

	return mux.open()
}
func (tm *TransportManager) IsMuxActive(name string) bool {
	return tm.muxConnManagers[name].status.Load() == int32(statusStarted)
}

func (tm *TransportManager) OpenClient(clientConfig config.ProxyClientConfig) (ClientTransport, error) {
	if clientConfig.Type == config.MuxTransport {
		return tm.openMuxTransport(clientConfig.MuxTransportName)
	}

	return &tcpClient{
		config: clientConfig.TCPClientSetting,
	}, nil
}

func (tm *TransportManager) OpenServer(serverConfig config.ProxyServerConfig) (ServerTransport, error) {
	if serverConfig.Type == config.MuxTransport {
		return tm.openMuxTransport(serverConfig.MuxTransportName)
	}

	return &tcpServer{
		config: serverConfig.TCPServerSetting,
	}, nil
}

func (tm *TransportManager) Start() error {
	for _, cm := range tm.muxConnManagers {
		if err := cm.start(); err != nil {
			return err
		}
	}

	return nil
}

func (tm *TransportManager) Stop() {
	for _, cm := range tm.muxConnManagers {
		cm.stop()
	}
}
