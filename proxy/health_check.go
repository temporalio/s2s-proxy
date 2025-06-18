package proxy

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/temporalio/s2s-proxy/metrics"
	"go.temporal.io/server/common/log"
	"net/http"
)

var (
	healthyGauge = metrics.DefaultGauge("health_check_success", "s2s-proxy service is healthy")
	healthyCount = metrics.DefaultCounter("health_check_success_count", "Number of healthy checks from s2s-proxy since service start")
)

func init() {
	prometheus.MustRegister(healthyGauge)
	prometheus.MustRegister(healthyCount)
}

type healthChecker struct {
	logger log.Logger
}

func (h *healthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: Check something here, and maybe log it
		healthyGauge.Set(1)
		healthyCount.Inc()
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}
}

func newHealthCheck(logger log.Logger) *healthChecker {
	return &healthChecker{
		logger: logger,
	}
}
