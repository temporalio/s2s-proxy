package proxy

import (
	"net/http"

	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/metrics"
)

type HealthChecker interface {
	createHandler() func(w http.ResponseWriter, r *http.Request)
}

// outboundHealthChecker contains references required to report whether the local Temporal server
// can make requests of the remote Temporal server
type outboundHealthChecker struct {
	isHealthy func() bool
	logger    log.Logger
}

func (h *outboundHealthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics.OutboundHealthCheckCount.Inc()
		if h.isHealthy() {
			metrics.OutboundIsHealthy.Set(1)
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			metrics.OutboundIsHealthy.Set(0)
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Outbound mux not established. Please try again"))
		}
	}
}

func newOutboundHealthCheck(isHealthy func() bool, logger log.Logger) HealthChecker {
	return &outboundHealthChecker{
		isHealthy: isHealthy,
		logger:    logger,
	}
}

type inboundHealthChecker struct {
	logger log.Logger
}

func (h *inboundHealthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// TODO: Check something here, and use it to set isHealthy to 0
		metrics.InboundIsHealthy.Set(1)
		metrics.InboundHealthCheckCount.Inc()
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}
}

func newInboundHealthCheck(logger log.Logger) HealthChecker {
	return &inboundHealthChecker{
		logger: logger,
	}
}
