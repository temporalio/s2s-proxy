package proxy

import (
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/transport"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type (
	ProxyServer struct {
		config       config.ProxyConfig
		opts         proxyOptions
		logger       log.Logger
		server       *TemporalAPIServer
		transManager *transport.TransportManager
		shutDownCh   chan struct{}
	}

	Proxy struct {
		config         config.S2SProxyConfig
		transManager   *transport.TransportManager
		outboundServer *ProxyServer
		inboundServer  *ProxyServer
	}

	proxyOptions struct {
		IsInbound bool
		Config    config.S2SProxyConfig
	}
)

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

func (ps *ProxyServer) startServer(
	serverTransport transport.ServerTransport,
	clientTransport transport.ClientTransport,
) error {
	cfg := ps.config
	opts := ps.opts
	logger := ps.logger

	serverOpts, err := makeServerOptions(logger, cfg, opts.IsInbound)
	if err != nil {
		return err
	}

	clientFactory := client.NewClientFactory(clientTransport, logger)
	ps.server = NewTemporalAPIServer(
		cfg.Name,
		cfg.Server,
		NewAdminServiceProxyServer(cfg.Name, cfg.Client, clientFactory, opts, logger),
		NewWorkflowServiceProxyServer(cfg.Name, cfg.Client, clientFactory, logger),
		serverOpts,
		serverTransport,
		logger,
	)

	ps.server.Start()
	return nil
}

func (ps *ProxyServer) stopServer() {
	if ps.server != nil {
		ps.server.Stop()
	}
}

func monitorClosable(closable transport.Closable, retryCh chan struct{}) {
	select {
	// Stop monitor if retryCh is closed
	case <-retryCh:
		return
	case <-closable.CloseChan():
		close(retryCh)
	}
}

func (ps *ProxyServer) start() error {
	serverConfig := ps.config.Server
	clientConfig := ps.config.Client

	go func() {
		for {
			clientTransport, err := ps.transManager.OpenClient(clientConfig)
			if err != nil {
				ps.logger.Error("Open client transport is failed", tag.Error(err))
				return
			}

			serverTransport, err := ps.transManager.OpenServer(serverConfig)
			if err != nil {
				ps.logger.Error("Open server transport is failed", tag.Error(err))
				return
			}

			ps.startServer(serverTransport, clientTransport)

			retryCh := make(chan struct{})

			if closable, ok := clientTransport.(transport.Closable); ok {
				go monitorClosable(closable, retryCh)
			}

			if closable, ok := serverTransport.(transport.Closable); ok {
				go monitorClosable(closable, retryCh)
			}

			select {
			case <-ps.shutDownCh:
				ps.stopServer()
				return
			case <-retryCh:
				// One of the closable transport is closed. Stop server and try re-open transport
				ps.stopServer()
			}
		}
	}()

	return nil
}

func (ps *ProxyServer) stop() {
	close(ps.shutDownCh)
}

func newProxyServer(
	cfg config.ProxyConfig,
	opts proxyOptions,
	transManager *transport.TransportManager,
	logger log.Logger,
) *ProxyServer {
	return &ProxyServer{
		config:       cfg,
		opts:         opts,
		transManager: transManager,
		logger:       logger,
		shutDownCh:   make(chan struct{}),
	}
}

func NewProxy(
	configProvider config.ConfigProvider,
	transManager *transport.TransportManager,
	logger log.Logger,
) (*Proxy, error) {
	s2sConfig := configProvider.GetS2SProxyConfig()
	proxy := &Proxy{
		config:       s2sConfig,
		transManager: transManager,
	}

	// Proxy consists of two grpc servers: inbound and outbound. The flow looks like the following:
	//    local server -> proxy(outbound) -> remote server
	//    local server <- proxy(inbound) <- remote server
	//
	// Here a remote server can be another proxy as well.
	//    server-a <-> proxy-a <-> proxy-b <-> server-b
	if s2sConfig.Outbound != nil {
		proxy.outboundServer = newProxyServer(
			*s2sConfig.Outbound,
			proxyOptions{
				IsInbound: false,
				Config:    s2sConfig,
			},
			transManager,
			logger,
		)
	}

	if s2sConfig.Inbound != nil {
		proxy.inboundServer = newProxyServer(
			*s2sConfig.Inbound,
			proxyOptions{
				IsInbound: true,
				Config:    s2sConfig,
			},
			transManager,
			logger,
		)
	}

	return proxy, nil
}

func (s *Proxy) Start() error {
	s.transManager.Start()
	if s.outboundServer != nil {
		if err := s.outboundServer.start(); err != nil {
			return err
		}
	}

	if s.inboundServer != nil {
		if err := s.inboundServer.start(); err != nil {
			return err
		}
	}

	return nil
}

func (s *Proxy) Stop() {
	if s.inboundServer != nil {
		s.inboundServer.stop()
	}
	if s.outboundServer != nil {
		s.outboundServer.stop()
	}

	s.transManager.Stop()
}
