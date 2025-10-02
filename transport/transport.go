package transport
//
//import (
//	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
//	"go.temporal.io/server/common/log"
//	"go.temporal.io/server/common/log/tag"
//	"google.golang.org/grpc"
//
//	"github.com/temporalio/s2s-proxy/config"
//	"github.com/temporalio/s2s-proxy/transport/mux"
//)
//
//type (
//	Closable interface {
//		CloseChan() <-chan struct{}
//		IsClosed() bool
//	}
//
//	TransportManager struct {
//		muxConnManagers map[string]mux.MultiMuxManager
//		logger          log.Logger
//	}
//)
//
//func NewTransportManager(
//	configProvider config.ConfigProvider,
//	logger log.Logger,
//) *TransportManager {
//
//	muxConnManagers := make(map[string]mux.MultiMuxManager)
//	s2sConfig := configProvider.GetS2SProxyConfig()
//	for _, cfg := range s2sConfig.MuxTransports {
//		muxMgr, err := mux.NewGRPCMuxManager(cfg, logger)
//		if err != nil {
//			logger.Fatal("Failed to configure mux manager", tag.Error(err))
//			panic(err)
//		}
//		muxConnManagers[cfg.Name] = muxMgr
//	}
//
//	return &TransportManager{
//		muxConnManagers: muxConnManagers,
//		logger:          logger,
//	}
//}
//func (tm *TransportManager) IsMuxActive(name string) bool {
//	//return tm.muxConnManagers[name].Load() == int32(statusStarted)
//	return tm.muxConnManagers[name].TryConnectionOrElse(func(*mux.SessionWithConn) any { return true }, false).(bool)
//}
//
//func (tm *TransportManager) OpenClient(clientConfig config.ProxyClientConfig) (ClientTransport, error) {
//	if clientConfig.Type == config.MuxTransport {
//		return tm.muxConnManagers[clientConfig.MuxTransportName], nil
//	}
//
//	return &tcpClient{
//		config: clientConfig.TCPClientSetting,
//	}, nil
//}
//
//func (tm *TransportManager) OpenServer(serverConfig config.ProxyServerConfig) (ServerTransport, error) {
//	if serverConfig.Type == config.MuxTransport {
//		return tm.muxConnManagers[serverConfig.MuxTransportName], nil
//	}
//
//	return &tcpServer{
//		config: serverConfig.TCPServerSetting,
//	}, nil
//}
//
//func (tm *TransportManager) Start() error {
//	for _, cm := range tm.muxConnManagers {
//		cm.Start()
//	}
//
//	return nil
//}
//
//func (tm *TransportManager) Stop() {
//	for _, cm := range tm.muxConnManagers {
//		cm.Close()
//	}
//}
