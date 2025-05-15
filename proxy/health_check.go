package proxy

import (
	"net/http"

	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/uber-go/tally/v4"
	"go.temporal.io/server/common/log"
)

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

type healthChecker struct {
	logger log.Logger
	scope  tally.Scope
}

func (h *healthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		h.logger.Info("Health Check")
		h.scope.Counter(metrics.HEALTH_CHECK_COUNT).Inc(1)
		healthCheckHandler(w, r)
	}
}

func newHealthCheck(logger log.Logger, scope tally.Scope) *healthChecker {
	return &healthChecker{
		logger: logger,
		scope:  scope,
	}
}
