package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
		inboundHealthCheckConfig  *config.HealthCheckConfig
		outboundHealthCheckConfig *config.HealthCheckConfig
		metricsConfig             *config.MetricsConfig
		clusterConnections        map[migrationId]*ClusterConnection
		inboundHealthCheckServer  *http.Server
		outboundHealthCheckServer *http.Server
		metricsServer             *http.Server
		logger                    log.Logger
	}
)

func NewProxy(configProvider config.ConfigProvider, logger log.Logger) *Proxy {
	s2sConfig := config.ToClusterConnConfig(configProvider.GetS2SProxyConfig())
	ctx, cancel := context.WithCancel(context.Background())
	proxy := &Proxy{
		lifetime:           ctx,
		cancel:             cancel,
		clusterConnections: make(map[migrationId]*ClusterConnection, len(s2sConfig.MuxTransports)),
		logger: log.NewThrottledLogger(
			logger,
			func() float64 {
				return s2sConfig.Logging.GetThrottleMaxRPS()
			},
		),
	}
	if (s2sConfig.Inbound == nil || s2sConfig.Outbound == nil) && len(s2sConfig.ClusterConnections) == 0 {
		panic(errors.New("cannot create proxy without inbound and outbound config"))
	}
	if s2sConfig.Metrics != nil {
		proxy.metricsConfig = s2sConfig.Metrics
	}
	// TODO: This is effectively 1 right now. We need to rewrite the config a bit to support multiple clusters
	for _, clusterCfg := range s2sConfig.ClusterConnections {
		cc, err := NewClusterConnection(ctx, clusterCfg, logger)
		if err != nil {
			logger.Fatal("Incorrectly configured Mux cluster connection", tag.Error(err), tag.NewStringTag("name", clusterCfg.Name))
			continue
		}
		proxy.clusterConnections[migrationId{clusterCfg.Name}] = cc
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
	if s.inboundHealthCheckConfig != nil {
		var err error
		healthFn := func() bool {
			// TODO: assumes only one mux right now. When there are multiple remotes, we may want to receive them all
			//       on the same port and do something different here
			for _, cc := range s.clusterConnections {
				if !cc.AcceptingInboundTraffic() {
					return false
				}
			}
			return true
		}
		if s.inboundHealthCheckServer, err = s.startHealthCheckHandler(s.lifetime, newInboundHealthCheck(healthFn, s.logger), *s.inboundHealthCheckConfig); err != nil {
			return err
		}
	} else {
		s.logger.Warn("Started up without inbound health check! Double-check the YAML config," +
			" it needs at least the following path: healthCheck.listenAddress")
	}

	if s.outboundHealthCheckConfig != nil {
		healthFn := func() bool {
			// TODO: assumes only one mux right now. When there are multiple remotes, some of them may be healthy
			//       and others not
			for _, cc := range s.clusterConnections {
				if !cc.AcceptingOutboundTraffic() {
					return false
				}
			}
			return true
		}
		var err error
		if s.outboundHealthCheckServer, err = s.startHealthCheckHandler(s.lifetime, newOutboundHealthCheck(healthFn, s.logger), *s.outboundHealthCheckConfig); err != nil {
			return err
		}
	} else {
		s.logger.Warn("Started up without outbound health check! Double-check the YAML config," +
			" it needs at least the following path: outboundHealthCheck.listenAddress")
	}

	if s.metricsConfig != nil {
		if err := s.startMetricsHandler(s.lifetime, *s.metricsConfig); err != nil {
			return err
		}
	} else {
		s.logger.Warn(`Started up without metrics! Double-check the YAML config,` +
			` it needs at least the following path: metrics.prometheus.listenAddress`)
	}

	for _, v := range s.clusterConnections {
		v.Start()
	}

	s.logger.Info(fmt.Sprintf("Started Proxy with the following config:\n%s", s.Describe()))

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

func (s *Proxy) Describe() string {
	sb := strings.Builder{}
	sb.WriteString("[proxy.Proxy with cluster connections:\n\t")
	for k, v := range s.clusterConnections {
		sb.WriteString(fmt.Sprintf("%s:", k.name))
		sb.WriteString(v.Describe())
		sb.WriteString("\n\t")
	}
	sb.WriteString("]")
	return sb.String()
}
