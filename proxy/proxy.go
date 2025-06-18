package proxy

import (
	"fmt"
	"net/http"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport"
	"github.com/uber-go/tally/v4"
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
		config            config.S2SProxyConfig
		transManager      *transport.TransportManager
		outboundServer    *ProxyServer
		inboundServer     *ProxyServer
		healthCheckServer *http.Server
		logger            log.Logger
		scope             tally.Scope
	}

	proxyOptions struct {
		IsInbound bool
		Config    config.S2SProxyConfig
	}
)

func makeServerOptions(
	logger log.Logger,
	cfg config.ProxyConfig,
	proxyOpts proxyOptions,
) ([]grpc.ServerOption, error) {
	unaryInterceptors := []grpc.UnaryServerInterceptor{}
	streamInterceptors := []grpc.StreamServerInterceptor{}

	var translators []interceptor.Translator
	if tln := proxyOpts.Config.NamespaceNameTranslation; tln.IsEnabled() {
		// NamespaceNameTranslator needs to be called before namespace access control so that
		// local name can be used in namespace allowed list.
		translators = append(translators,
			interceptor.NewNamespaceNameTranslator(tln.ToMaps(proxyOpts.IsInbound)))
	}

	if tln := proxyOpts.Config.SearchAttributeTranslation; tln.IsEnabled() {
		logger.Info("search attribute translation enabled", tag.NewAnyTag("mappings", tln.Mappings))
		translators = append(translators,
			interceptor.NewSearchAttributeTranslator(tln.ToMaps(proxyOpts.IsInbound)))
	}

	if len(translators) > 0 {
		tr := interceptor.NewTranslationInterceptor(logger, translators)
		unaryInterceptors = append(unaryInterceptors, tr.Intercept)
		streamInterceptors = append(streamInterceptors, tr.InterceptStream)
	}

	if proxyOpts.IsInbound && cfg.ACLPolicy != nil {
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

func (ps *ProxyServer) makeNamespaceACL() *auth.AccessControl {
	if ps.opts.IsInbound && ps.config.ACLPolicy != nil {
		return auth.NewAccesControl(ps.config.ACLPolicy.AllowedNamespaces)
	}
	return nil
}

func (ps *ProxyServer) startServer(
	serverTransport transport.ServerTransport,
	clientTransport transport.ClientTransport,
) error {
	cfg := ps.config
	opts := ps.opts
	logger := ps.logger

	serverOpts, err := makeServerOptions(logger, cfg, opts)
	if err != nil {
		return err
	}

	clientFactory := client.NewClientFactory(clientTransport, logger)
	ps.server = NewTemporalAPIServer(
		cfg.Name,
		cfg.Server,
		NewAdminServiceProxyServer(cfg.Name, cfg.Client, clientFactory, opts, logger),
		NewWorkflowServiceProxyServer(cfg.Name, cfg.Client, clientFactory, ps.makeNamespaceACL(), logger),
		serverOpts,
		serverTransport,
		logger,
	)

	ps.logger.Info(fmt.Sprintf("Starting ProxyServer %s with ServerConfig: %v, ClientConfig: %v", cfg.Name, cfg.Server, cfg.Client))
	ps.server.Start()
	return nil
}

func (ps *ProxyServer) stopServer() {
	if ps.server != nil {
		ps.server.Stop()
	}
}

func monitorClosable(closable transport.Closable, retryCh chan struct{}, shutDownCh <-chan struct{}) {
	select {
	case <-shutDownCh:
		return
	// Stop monitor if retryCh is already closed
	case <-retryCh:
		return
	case <-closable.CloseChan():
		// TODO: avoid retryCh to be closed twice.
		close(retryCh)
	}
}

func (ps *ProxyServer) start() error {
	serverConfig := ps.config.Server
	clientConfig := ps.config.Client

	go func() {
		for {
			// If using mux transport underneath, Open call will be blocked until
			// underlying connection is established.
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

			if err := ps.startServer(serverTransport, clientTransport); err != nil {
				ps.logger.Error("Failed to start server", tag.Error(err))
				return
			}

			retryCh := make(chan struct{})
			if closable, ok := clientTransport.(transport.Closable); ok {
				go monitorClosable(closable, retryCh, ps.shutDownCh)
			}

			if closable, ok := serverTransport.(transport.Closable); ok {
				go monitorClosable(closable, retryCh, ps.shutDownCh)
			}

			select {
			case <-ps.shutDownCh:
				ps.stopServer()
				return
			case <-retryCh:
				// If any closable transport is closed, try to restart the proxy server.
				ps.stopServer()
			}
		}
	}()

	return nil
}

func (ps *ProxyServer) stop() {
	ps.logger.Info("Stop ProxyServer")
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
	scope tally.Scope,
) *Proxy {
	s2sConfig := configProvider.GetS2SProxyConfig()
	proxy := &Proxy{
		config:       s2sConfig,
		transManager: transManager,
		logger:       logger,
		scope:        scope,
	}

	scope.Counter(metrics.PROXY_START_COUNT).Inc(1)

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

	return proxy
}

func (s *Proxy) startHealthCheckHandler(cfg config.HealthCheckConfig) error {
	if cfg.Protocol != config.HTTP {
		return fmt.Errorf("Not supported health check protocol %s", cfg.Protocol)
	}

	// Define the server and its settings
	s.healthCheckServer = &http.Server{
		Addr:    cfg.ListenAddress,
		Handler: nil, // Default HTTP handler (using http.HandleFunc registrations)
	}

	checker := newHealthCheck(s.logger, s.scope)

	// Register the health check endpoint
	http.HandleFunc("/health", checker.createHandler())

	go func() {
		s.logger.Info("Starting health check server", tag.Address(cfg.ListenAddress))
		if err := s.healthCheckServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Error starting server: %v\n", tag.Error(err))
		}
	}()

	return nil
}

func (s *Proxy) Start() error {
	if s.config.HealthCheck != nil {
		if err := s.startHealthCheckHandler(*s.config.HealthCheck); err != nil {
			return err
		}
	}

	if err := s.transManager.Start(); err != nil {
		return err
	}

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
	if s.healthCheckServer != nil {
		// Close without waiting for in-flight requests to complete.
		s.healthCheckServer.Close()
	}

	if s.inboundServer != nil {
		s.inboundServer.stop()
	}
	if s.outboundServer != nil {
		s.outboundServer.stop()
	}
	s.transManager.Stop()
}
