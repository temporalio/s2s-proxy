package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	migrationId struct {
		name string
		// Needs some config revision before uncommenting:
		//accountId string
	}
	Proxy struct {
		lifetime                  context.Context
		cancel                    context.CancelFunc
		config                    config.S2SProxyConfig
		clusterConnections        map[migrationId]*ClusterConnection
		inboundHealthCheckServer  *http.Server
		outboundHealthCheckServer *http.Server
		metricsServer             *http.Server
		logger                    log.Logger
	}
)

func NewProxy(configProvider config.ConfigProvider, logger log.Logger) *Proxy {
	s2sConfig := configProvider.GetS2SProxyConfig()
	ctx, cancel := context.WithCancel(context.Background())
	proxy := &Proxy{
		lifetime:           ctx,
		cancel:             cancel,
		config:             s2sConfig,
		clusterConnections: make(map[migrationId]*ClusterConnection, len(s2sConfig.MuxTransports)+1),
		logger: log.NewThrottledLogger(
			logger,
			func() float64 {
				return s2sConfig.Logging.GetThrottleMaxRPS()
			},
		),
	}
	// TODO: This is effectively 1 right now. We need to rewrite the config a bit to support multiple clusters
	for _, muxCfg := range s2sConfig.MuxTransports {
		cc, err := NewMuxClusterConnection(ctx, muxCfg, *s2sConfig.Inbound, *s2sConfig.Outbound,
			s2sConfig.NamespaceNameTranslation, s2sConfig.SearchAttributeTranslation, logger)
		if err != nil {
			logger.Fatal("Incorrectly configured Mux cluster connection", tag.Error(err), tag.NewStringTag("name", muxCfg.Name))
			continue
		}
		proxy.clusterConnections[migrationId{muxCfg.Name}] = cc
	}
	if len(s2sConfig.MuxTransports) == 0 {
		cc, err := NewTCPClusterConnection(ctx, *s2sConfig.Inbound, *s2sConfig.Outbound,
			s2sConfig.NamespaceNameTranslation, s2sConfig.SearchAttributeTranslation, logger)
		if err != nil {
			logger.Fatal("Incorrectly configured TCP cluster connection", tag.Error(err))
			panic(err)
		}
		proxy.clusterConnections[migrationId{"default"}] = cc
	}

	metrics.NewProxyCount.Inc()
	return proxy
}

func (s *Proxy) startHealthCheckHandler(lifetime context.Context, healthChecker HealthChecker, cfg config.HealthCheckConfig) (*http.Server, error) {
	if cfg.Protocol != config.HTTP {
		return nil, fmt.Errorf("unsupported health check protocol %s", cfg.Protocol)
	}

	// Set up the handler. Avoid the global ServeMux so that we can create N of these in unit test suites
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/health", healthChecker.createHandler())
	// Define the server and its settings
	healthCheckServer := &http.Server{
		Addr:    cfg.ListenAddress,
		Handler: serveMux,
	}

	go func() {
		s.logger.Info("Starting health check server", tag.Address(cfg.ListenAddress))
		if err := healthCheckServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("Error starting server", tag.Error(err))
		}
	}()
	context.AfterFunc(lifetime, func() {
		_ = healthCheckServer.Close()
	})
	return healthCheckServer, nil
}

func (s *Proxy) startMetricsHandler(lifetime context.Context, cfg config.MetricsConfig) error {
	// Why not use the global ServeMux? So that it can be used in unit tests
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.NewMetricsHandler(s.logger))
	s.metricsServer = &http.Server{
		Addr:    cfg.Prometheus.ListenAddress,
		Handler: mux,
	}

	go func() {
		s.logger.Info("Starting metrics server", tag.Address(cfg.Prometheus.ListenAddress))
		if err := s.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("Error starting server", tag.Error(err))
		}
	}()
	context.AfterFunc(lifetime, func() {
		_ = s.metricsServer.Close()
	})
	return nil
}

func (s *Proxy) Start() error {
	if s.config.HealthCheck != nil {
		var err error
		if s.inboundHealthCheckServer, err = s.startHealthCheckHandler(s.lifetime, newInboundHealthCheck(s.logger), *s.config.HealthCheck); err != nil {
			return err
		}
	} else {
		s.logger.Warn("Started up without inbound health check! Double-check the YAML config," +
			" it needs at least the following path: healthCheck.listenAddress")
	}

	if s.config.OutboundHealthCheck != nil {
		healthFn := func() bool {
			// TODO: assumes only one mux right now. When there are multiple outbounds, some of them may be healthy
			//       and others not
			for _, cc := range s.clusterConnections {
				if !cc.RemoteUsable() {
					return false
				}
			}
			return true
		}
		var err error
		if s.outboundHealthCheckServer, err = s.startHealthCheckHandler(s.lifetime, newOutboundHealthCheck(healthFn, s.logger), *s.config.OutboundHealthCheck); err != nil {
			return err
		}
	} else {
		s.logger.Warn("Started up without outbound health check! Double-check the YAML config," +
			" it needs at least the following path: outboundHealthCheck.listenAddress")
	}

	if s.config.Metrics != nil {
		if err := s.startMetricsHandler(s.lifetime, *s.config.Metrics); err != nil {
			return err
		}
	} else {
		s.logger.Warn(`Started up without metrics! Double-check the YAML config,` +
			` it needs at least the following path: metrics.prometheus.listenAddress`)
	}

	for _, v := range s.clusterConnections {
		v.Start()
	}

	return nil
}

func (s *Proxy) Stop() {
	// All parts of the Proxy watch the "lifetime" context. Cancelling it will close all components
	// where necessary
	s.cancel()
}

func (s *Proxy) Done() <-chan struct{} {
	return s.lifetime.Done()
}
