package proxy

import (
	"net/http"

	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/metrics"
)

type HealthChecker interface {
	createHandler() func(w http.ResponseWriter, r *http.Request)
}

// remoteHealthChecker reports whether the local Temporal server can make requests of the remote Temporal server.
type remoteHealthChecker struct {
	isHealthy func() bool
	logger    log.Logger
}

func (h *remoteHealthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics.LBHealthCheckCount.WithLabelValues("outbound").Inc()
		w.Header().Set("Content-Type", "text/plain")
		if h.isHealthy() {
			metrics.LBHealthSuccessCount.WithLabelValues("outbound").Inc()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Outbound mux not established. Please try again"))
		}
	}
}

func newRemoteHealthCheck(isHealthy func() bool, logger log.Logger) HealthChecker {
	return &remoteHealthChecker{
		isHealthy: isHealthy,
		logger:    logger,
	}
}

type localHealthChecker struct {
	isHealthy func() bool
	logger    log.Logger
}

func (h *localHealthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics.LBHealthCheckCount.WithLabelValues("inbound").Inc()
		w.Header().Set("Content-Type", "text/plain")
		if h.isHealthy() {
			metrics.LBHealthSuccessCount.WithLabelValues("inbound").Inc()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Mux capacity is full. Please try again"))
		}
	}
}

func newLocalHealthCheck(isHealthy func() bool, logger log.Logger) HealthChecker {
	return &localHealthChecker{
		isHealthy: isHealthy,
		logger:    logger,
	}
}
