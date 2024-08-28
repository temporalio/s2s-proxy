package proxy

import (
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/interceptor"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
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

	proxy := Proxy{
		configProvider: configProvider,
	}

	if s2sConfig.Outbound != nil {
		serverOpts, err := makeServerOptions(logger, *s2sConfig.Outbound)
		if err != nil {
			logger.Fatal("Failed to construct grpc server options", tag.Error(err), tag.NewStringTag("name", s2sConfig.Outbound.Name))
			return nil
		}

		proxy.outboundServer = NewTemporalAPIServer(
			s2sConfig.Outbound.Name,
			s2sConfig.Outbound.Server,
			NewAdminServiceProxyServer(*s2sConfig.Outbound, clientFactory, logger),
			NewWorkflowServiceProxyServer(*s2sConfig.Outbound, clientFactory, logger),
			serverOpts,
			logger,
		)
	}

	if s2sConfig.Inbound != nil {
		serverOpts, err := makeServerOptions(logger, *s2sConfig.Inbound)
		if err != nil {
			logger.Fatal("Failed to construct grpc server options", tag.Error(err), tag.NewStringTag("name", s2sConfig.Inbound.Name))
			return nil
		}

		proxy.inboundServer = NewTemporalAPIServer(
			s2sConfig.Inbound.Name,
			s2sConfig.Inbound.Server,
			NewAdminServiceProxyServer(*s2sConfig.Inbound, clientFactory, logger),
			NewWorkflowServiceProxyServer(*s2sConfig.Inbound, clientFactory, logger),
			serverOpts,
			logger,
		)
	}

	return &proxy
}

func makeServerOptions(logger log.Logger, cfg config.ProxyConfig) ([]grpc.ServerOption, error) {
	interceptors := []grpc.UnaryServerInterceptor{}
	if len(cfg.NamespaceNameTranslation.Mappings) > 0 {
		interceptors = append(interceptors, interceptor.NewNamespaceNameTranslator(logger, cfg).Intercept)
	}

	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(interceptors...),
	}

	if cfg.Server.TLS.IsEnabled() {
		tlsConfig, err := encryption.GetServerTLSConfig(cfg.Server.TLS)
		if err != nil {
			return opts, err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	return opts, nil
}

func (s *Proxy) Start() error {
	if s.outboundServer != nil {
		if err := s.outboundServer.Start(); err != nil {
			return err
		}
	}

	if s.inboundServer != nil {
		if err := s.inboundServer.Start(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Proxy) Stop() {
	if s.inboundServer != nil {
		s.inboundServer.Stop()
	}
	if s.outboundServer != nil {
		s.outboundServer.Stop()
	}
}
