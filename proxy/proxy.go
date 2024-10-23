package proxy

import (
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/transport"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type (
	Proxy struct {
		config           config.S2SProxyConfig
		transportManager transport.TransportManager
		outboundServer   *TemporalAPIServer
		inboundServer    *TemporalAPIServer
	}

	proxyOptions struct {
		IsInbound bool
		Config    config.S2SProxyConfig
	}
)

func NewProxy(
	configProvider config.ConfigProvider,
	transportManager transport.TransportManager,
	logger log.Logger,
) (*Proxy, error) {
	s2sConfig := configProvider.GetS2SProxyConfig()
	var err error

	// Establish underlying connection first before start proxy server.
	if err := transportManager.Start(); err != nil {
		return nil, err
	}

	proxy := Proxy{
		config:           s2sConfig,
		transportManager: transportManager,
	}

	// Proxy consists of two grpc servers: inbound and outbound. The flow looks like the following:
	//    local server -> proxy(outbound) -> remote server
	//    local server <- proxy(inbound) <- remote server
	//
	// Here a remote server can be another proxy as well.
	//    server-a <-> proxy-a <-> proxy-b <-> server-b
	if s2sConfig.Outbound != nil {
		if proxy.outboundServer, err = proxy.createServer(*s2sConfig.Outbound, logger, proxyOptions{
			IsInbound: false,
			Config:    s2sConfig,
		}); err != nil {
			return nil, err
		}
	}

	if s2sConfig.Inbound != nil {
		if proxy.inboundServer, err = proxy.createServer(*s2sConfig.Inbound, logger, proxyOptions{
			IsInbound: true,
			Config:    s2sConfig,
		}); err != nil {
			return nil, err
		}
	}

	return &proxy, nil
}

func makeServerOptions(logger log.Logger, cfg config.ProxyConfig, isInbound bool) ([]grpc.ServerOption, error) {
	unaryInterceptors := []grpc.UnaryServerInterceptor{}
	streamInterceptors := []grpc.StreamServerInterceptor{}

	if len(cfg.NamespaceNameTranslation.Mappings) > 0 {
		// NamespaceNameTranslator needs to be called before namespace access control so that
		// local name can be used in namespace allowed list.
		unaryInterceptors = append(unaryInterceptors, interceptor.NewNamespaceNameTranslator(logger, cfg, isInbound).Intercept)
	}

	if isInbound && cfg.ACLPolicy != nil {
		aclInterceptor := interceptor.NewAccessControlInterceptor(logger, cfg.ACLPolicy)
		unaryInterceptors = append(unaryInterceptors, aclInterceptor.Intercept)
		streamInterceptors = append(streamInterceptors, aclInterceptor.StreamIntercept)
	}

	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
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

func (s *Proxy) createServer(cfg config.ProxyConfig, logger log.Logger, opts proxyOptions) (*TemporalAPIServer, error) {
	serverOpts, err := makeServerOptions(logger, cfg, opts.IsInbound)
	if err != nil {
		return nil, err
	}

	serverTransport, err := s.transportManager.CreateServerTransport(cfg.Server)
	if err != nil {
		return nil, err
	}

	clientFactory := client.NewClientFactory(s.transportManager, logger)
	return NewTemporalAPIServer(
		cfg.Name,
		cfg.Server,
		NewAdminServiceProxyServer(cfg.Name, cfg.Client, clientFactory, opts, logger),
		NewWorkflowServiceProxyServer(cfg.Name, cfg.Client, clientFactory, logger),
		serverOpts,
		serverTransport,
		logger,
	), nil
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

	s.transportManager.Stop()
}
