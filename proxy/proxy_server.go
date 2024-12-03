package proxy

import (
	"fmt"
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

type ProxyServer struct {
	config       config.ProxyConfig
	opts         proxyOptions
	logger       log.Logger
	server       *TemporalAPIServer
	transManager *transport.TransportManager
	shutDownCh   chan struct{}
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

	ps.logger.Info(fmt.Sprintf("Starting ProxyServer %s with ServerConfig: %v, ClientConfig: %v", cfg.Name, cfg.Server, cfg.Client))
	ps.server.Start()
	return nil
}

func (ps *ProxyServer) stopServer() {
	if ps.server != nil {
		ps.server.Stop()
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

			ps.startServer(serverTransport, clientTransport)

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
