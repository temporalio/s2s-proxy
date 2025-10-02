package proxy

//
//import (
//	"fmt"
//
//	"github.com/prometheus/client_golang/prometheus"
//	"github.com/temporalio/s2s-proxy/auth"
//	"github.com/temporalio/s2s-proxy/client"
//	"github.com/temporalio/s2s-proxy/config"
//	"github.com/temporalio/s2s-proxy/metrics"
//	"github.com/temporalio/s2s-proxy/transport"
//	"go.temporal.io/server/common/log"
//	"go.temporal.io/server/common/log/tag"
//)
//
//type (
//	ProxyServer struct {
//		config       config.ProxyConfig
//		opts         proxyOptions
//		logger       log.Logger
//		server       *TemporalAPIServer
//		transManager *transport.TransportManager
//		metricLabels prometheus.Labels
//		shutDownCh   chan struct{}
//	}
//
//	proxyOptions struct {
//		IsInbound bool
//		Config    config.S2SProxyConfig
//	}
//)
//
//func monitorClosable(closable transport.Closable, retryCh chan struct{}, shutDownCh <-chan struct{}) {
//	select {
//	case <-shutDownCh:
//		return
//	// Stop monitor if retryCh is already closed
//	case <-retryCh:
//		return
//	case <-closable.CloseChan():
//		// TODO: avoid retryCh to be closed twice.
//		close(retryCh)
//	}
//}
//
//func (ps *ProxyServer) makeNamespaceACL() *auth.AccessControl {
//	if ps.opts.IsInbound && ps.config.ACLPolicy != nil {
//		return auth.NewAccesControl(ps.config.ACLPolicy.AllowedNamespaces)
//	}
//	return nil
//}
//
//func (ps *ProxyServer) startServer(
//	serverTransport transport.ServerTransport,
//	clientTransport transport.ClientTransport,
//) error {
//	cfg := ps.config
//	opts := ps.opts
//	logger := ps.logger
//
//	serverOpts, err := makeServerOptions(logger, cfg, opts)
//	if err != nil {
//		return err
//	}
//
//	clientMetrics := metrics.GRPCOutboundClientMetrics
//	if ps.opts.IsInbound {
//		clientMetrics = metrics.GRPCInboundClientMetrics
//	}
//
//	clientFactory := client.NewClientFactory(clientTransport, clientMetrics, logger)
//	ps.server = NewTemporalAPIServer(
//		cfg.Name,
//		cfg.Server,
//		NewAdminServiceProxyServer(cfg.Name, cfg.Client, clientFactory, opts, logger),
//		NewWorkflowServiceProxyServer(cfg.Name, cfg.Client, clientFactory, ps.makeNamespaceACL(), logger),
//		serverOpts,
//		serverTransport,
//		logger,
//	)
//
//	ps.logger.Info(fmt.Sprintf("Starting ProxyServer %s with ServerConfig: %v, ClientConfig: %v", cfg.Name, cfg.Server, cfg.Client))
//	ps.server.Start()
//	return nil
//}
//
//func (ps *ProxyServer) stopServer() {
//	if ps.server != nil {
//		ps.server.Stop()
//	}
//}
//func (opts *proxyOptions) directionLabel() string {
//	directionValue := "outbound"
//	if opts.IsInbound {
//		directionValue = "inbound"
//	}
//	return directionValue
//}
//
//func (ps *ProxyServer) start() error {
//	serverConfig := ps.config.Server
//	clientConfig := ps.config.Client
//
//	go func() {
//		for {
//			metrics.ProxyServiceCreated.With(ps.metricLabels).Inc()
//			// If using mux transport underneath, Open call will be blocked until
//			// underlying connection is established.
//			// Also note: GRPC requires the client interceptors (like metrics) to be defined on the transport, not on the client.
//			clientTransport, err := ps.transManager.OpenClient(clientConfig)
//			if err != nil {
//				ps.logger.Error("Open client transport is failed", tag.Error(err))
//				return
//			}
//
//			serverTransport, err := ps.transManager.OpenServer(serverConfig)
//			if err != nil {
//				ps.logger.Error("Open server transport is failed", tag.Error(err))
//				return
//			}
//
//			if err := ps.startServer(serverTransport, clientTransport); err != nil {
//				ps.logger.Error("Failed to start server", tag.Error(err))
//				return
//			}
//
//			retryCh := make(chan struct{})
//			if closable, ok := clientTransport.(transport.Closable); ok {
//				go monitorClosable(closable, retryCh, ps.shutDownCh)
//			}
//
//			if closable, ok := serverTransport.(transport.Closable); ok {
//				go monitorClosable(closable, retryCh, ps.shutDownCh)
//			}
//
//			select {
//			case <-ps.shutDownCh:
//				metrics.ProxyServiceStopped.With(ps.metricLabels).Inc()
//				ps.stopServer()
//				return
//			case <-retryCh:
//				// If any closable transport is closed, try to restart the proxy server.
//				metrics.ProxyServiceRestarted.With(ps.metricLabels).Inc()
//				ps.stopServer()
//			}
//		}
//	}()
//
//	return nil
//}
//
//func (ps *ProxyServer) stop() {
//	ps.logger.Info("Stop ProxyServer")
//	close(ps.shutDownCh)
//}
//
//func newProxyServer(
//	cfg config.ProxyConfig,
//	opts proxyOptions,
//	transManager *transport.TransportManager,
//	logger log.Logger,
//) *ProxyServer {
//	return &ProxyServer{
//		config:       cfg,
//		opts:         opts,
//		transManager: transManager,
//		logger:       logger,
//		metricLabels: prometheus.Labels{"direction": opts.directionLabel()},
//		shutDownCh:   make(chan struct{}),
//	}
//}
