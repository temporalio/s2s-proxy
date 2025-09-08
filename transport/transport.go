package transport

import (
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/temporalio/s2s-proxy/transport/mux"
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
		muxConnManagers map[string]mux.MuxManager
		logger          log.Logger
	}
)

func NewTransportManager(
	configProvider config.ConfigProvider,
	logger log.Logger,
) *TransportManager {

	muxConnManagers := make(map[string]mux.MuxManager)
	s2sConfig := configProvider.GetS2SProxyConfig()
	for _, cfg := range s2sConfig.MuxTransports {
		muxConnManagers[cfg.Name] = mux.NewMuxManager(cfg, logger)
	}

	return &TransportManager{
		muxConnManagers: muxConnManagers,
		logger:          logger,
	}
}
func (tm *TransportManager) IsMuxActive(name string) bool {
	//return tm.muxConnManagers[name].Load() == int32(statusStarted)
	return mux.TryConnectionOrElse(tm.muxConnManagers[name], func(*mux.SessionWithConn) bool { return true }, false)
}

func (tm *TransportManager) OpenClient(clientConfig config.ProxyClientConfig) (ClientTransport, error) {
	if clientConfig.Type == config.MuxTransport {
		return tm.muxConnManagers[clientConfig.MuxTransportName], nil
	}

	return &tcpClient{
		config: clientConfig.TCPClientSetting,
	}, nil
}

func (tm *TransportManager) OpenServer(serverConfig config.ProxyServerConfig) (ServerTransport, error) {
	if serverConfig.Type == config.MuxTransport {
		return tm.muxConnManagers[serverConfig.MuxTransportName], nil
	}

	return &tcpServer{
		config: serverConfig.TCPServerSetting,
	}, nil
}

func (tm *TransportManager) Start() error {
	for _, cm := range tm.muxConnManagers {
		if err := cm.Start(); err != nil {
			return err
		}
	}

	return nil
}

func (tm *TransportManager) Stop() {
	for _, cm := range tm.muxConnManagers {
		cm.Close()
	}
}
