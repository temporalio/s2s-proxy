package proxy

import (
	"fmt"
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/proxy/cadence"
	"github.com/temporalio/s2s-proxy/transport"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

type CadenceProxyServer struct {
	config       config.ProxyConfig
	opts         proxyOptions
	logger       log.Logger
	server       *cadence.CadenceAPIServer
	transManager *transport.TransportManager
	shutDownCh   chan struct{}
}

func newCadenceProxyServer(
	cfg config.ProxyConfig,
	opts proxyOptions,
	transManager *transport.TransportManager,
	logger log.Logger,
) *CadenceProxyServer {
	return &CadenceProxyServer{
		config:       cfg,
		opts:         opts,
		transManager: transManager,
		logger:       logger,
		shutDownCh:   make(chan struct{}),
	}
}

func (ps *CadenceProxyServer) startServer(
	clientTransport transport.ClientTransport,
) error {
	cfg := ps.config

	clientFactory := client.NewClientFactory(clientTransport, ps.logger)
	ps.server = cadence.NewCadenceAPIServer(ps.logger, cfg.Client, clientFactory, ps.config.Server)

	ps.logger.Info(fmt.Sprintf("Starting Cadence ProxyServer %s with ServerConfig: %v, ClientConfig: %v", cfg.Name, cfg.Server, cfg.Client))
	ps.server.Start()
	return nil
}

func (ps *CadenceProxyServer) stopServer() {
	if ps.server != nil {
		ps.server.Stop()
	}
}

func (ps *CadenceProxyServer) start() error {
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

			ps.startServer(clientTransport)

			retryCh := make(chan struct{})
			if closable, ok := clientTransport.(transport.Closable); ok {
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

func (ps *CadenceProxyServer) stop() {
	ps.logger.Info("Stop ProxyServer")
	close(ps.shutDownCh)
}
