package proxy

import (
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/interceptor"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
)

type (
	Proxy struct {
		configProvider config.ConfigProvider
		outboundServer *TemporalAPIServer
		inboundServer  *TemporalAPIServer
	}
)

func NewProxy(
	configProvider config.ConfigProvider,
	logger log.Logger,
	clientFactory client.ClientFactory,
) *Proxy {
	s2sConfig := configProvider.GetS2SProxyConfig()

	// Proxy consists of two grpc servers: inbound and outbound. The flow looks like the following:
	//    local server -> proxy(outbound) -> remote server
	//    local server <- proxy(inbound) <- remote server
	//
	// Here a remote server can be another proxy as well.
	//    server-a <-> proxy-a <-> proxy-b <-> server-b

	return &Proxy{
		configProvider: configProvider,
		outboundServer: NewTemporalAPIServer(
			s2sConfig.Outbound.Name,
			s2sConfig.Outbound.Server,
			NewAdminServiceProxyServer(s2sConfig.Outbound, clientFactory, logger),
			makeServerOptions(logger, s2sConfig.Outbound),
			logger,
		),
		inboundServer: NewTemporalAPIServer(
			s2sConfig.Inbound.Name,
			s2sConfig.Inbound.Server,
			NewAdminServiceProxyServer(s2sConfig.Inbound, clientFactory, logger),
			makeServerOptions(logger, s2sConfig.Inbound),
			logger,
		),
	}
}

func makeServerOptions(logger log.Logger, cfg config.ProxyConfig) []grpc.ServerOption {
	interceptors := []grpc.UnaryServerInterceptor{}
	if len(cfg.NamespaceNameTranslation.Mappings) > 0 {
		interceptors = append(interceptors, interceptor.NewNamespaceNameTranslator(logger, cfg).Intercept)
	}
	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(interceptors...),
	}
	return opts
}

func (s *Proxy) Start() error {
	if err := s.outboundServer.Start(); err != nil {
		return err
	}

	if err := s.inboundServer.Start(); err != nil {
		return err
	}

	return nil
}

func (s *Proxy) Stop() {
	s.inboundServer.Stop()
	s.outboundServer.Stop()
}
