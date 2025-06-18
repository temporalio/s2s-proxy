package proxy

import (
	"github.com/temporalio/s2s-proxy/metrics"
	"go.temporal.io/server/common/log"
	"net/http"
)

type healthChecker struct {
	logger log.Logger
}

func (h *healthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: Check something here, and use it to set isHealthy to 0
		metrics.HealthCheckIsHealthy.Set(1)
		metrics.HealthCheckHealthyCount.Inc()
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
