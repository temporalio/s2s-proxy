package proxy

import (
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport"
	"go.temporal.io/server/common/log"
)

type (
	Proxy struct {
		config         config.S2SProxyConfig
		transManager   *transport.TransportManager
		outboundServer *ProxyServer
		inboundServer  *ProxyServer
		cadenceServer  *CadenceProxyServer
		logger         log.Logger
	}

	proxyOptions struct {
		IsInbound bool
		Config    config.S2SProxyConfig
	}
)

func NewProxy(
	configProvider config.ConfigProvider,
	transManager *transport.TransportManager,
	logger log.Logger,
) *Proxy {
	s2sConfig := configProvider.GetS2SProxyConfig()
	proxy := &Proxy{
		config:       s2sConfig,
		transManager: transManager,
		logger:       logger,
	}

	// Proxy consists of two grpc servers: inbound and outbound. The flow looks like the following:
	//    local server -> proxy(outbound) -> remote server
	//    local server <- proxy(inbound) <- remote server
	//
	// Here a remote server can be another proxy as well.
	//    server-a <-> proxy-a <-> proxy-b <-> server-b
	if s2sConfig.Outbound != nil {
		if s2sConfig.Outbound.Server.Type == config.CadenceTransport {
			proxy.cadenceServer = newCadenceProxyServer(
				*s2sConfig.Outbound,
				proxyOptions{
					IsInbound: false,
					Config:    s2sConfig,
				},
				transManager,
				logger,
			)
		} else {
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

func (s *Proxy) Start() error {
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

	if s.cadenceServer != nil {
		if err := s.cadenceServer.start(); err != nil {
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
