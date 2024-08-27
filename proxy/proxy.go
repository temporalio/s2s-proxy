package proxy

import (
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/log"
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
	grpcServerOptions GrpcServerOptions,
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
			grpcServerOptions,
			logger,
		),
		inboundServer: NewTemporalAPIServer(
			s2sConfig.Inbound.Name,
			s2sConfig.Inbound.Server,
			NewAdminServiceProxyServer(s2sConfig.Inbound, clientFactory, logger),
			grpcServerOptions,
			logger,
		),
	}
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
