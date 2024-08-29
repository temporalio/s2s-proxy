package proxy

import (
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/interceptor"
	"go.temporal.io/server/common/log"
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

func createProxy(cfg config.ProxyConfig, logger log.Logger, clientFactory client.ClientFactory) (*TemporalAPIServer, error) {
	serverOpts, err := makeServerOptions(logger, cfg)
	if err != nil {
		return nil, err
	}

	return NewTemporalAPIServer(
		cfg.Name,
		cfg.Server,
		NewAdminServiceProxyServer(cfg.Name, cfg.Client, clientFactory, logger),
		NewWorkflowServiceProxyServer(cfg, clientFactory, logger),
		serverOpts,
		logger,
	), nil
}

func NewProxy(
	configProvider config.ConfigProvider,
	logger log.Logger,
	clientFactory client.ClientFactory,
) (*Proxy, error) {
	s2sConfig := configProvider.GetS2SProxyConfig()
	var err error

	proxy := Proxy{
		configProvider: configProvider,
	}

	// Proxy consists of two grpc servers: inbound and outbound. The flow looks like the following:
	//    local server -> proxy(outbound) -> remote server
	//    local server <- proxy(inbound) <- remote server
	//
	// Here a remote server can be another proxy as well.
	//    server-a <-> proxy-a <-> proxy-b <-> server-b
	if s2sConfig.Outbound != nil {
		if proxy.outboundServer, err = createProxy(*s2sConfig.Outbound, logger, clientFactory); err != nil {
			return nil, err
		}
	}

	if s2sConfig.Inbound != nil {
		if proxy.inboundServer, err = createProxy(*s2sConfig.Inbound, logger, clientFactory); err != nil {
			return nil, err
		}
	}

	return &proxy, nil
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
