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
	reg       *metrics.Registry
}

func (h *outboundHealthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		h.reg.LBHealthCheckCount.WithLabelValues("outbound").Inc()
		w.Header().Set("Content-Type", "text/plain")
		if h.isHealthy() {
			h.reg.LBHealthSuccessCount.WithLabelValues("outbound").Inc()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Outbound mux not established. Please try again"))
		}
	}
}

func newOutboundHealthCheck(isHealthy func() bool, logger log.Logger, reg *metrics.Registry) HealthChecker {
	return &outboundHealthChecker{
		isHealthy: isHealthy,
		logger:    logger,
		reg:       reg,
	}
}

type inboundHealthChecker struct {
	isHealthy func() bool
	logger    log.Logger
	reg       *metrics.Registry
}

func (h *inboundHealthChecker) createHandler() func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		h.reg.LBHealthCheckCount.WithLabelValues("inbound").Inc()
		w.Header().Set("Content-Type", "text/plain")
		if h.isHealthy() {
			h.reg.LBHealthSuccessCount.WithLabelValues("inbound").Inc()
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("Mux capacity is full. Please try again"))
		}
	}
}

func newInboundHealthCheck(isHealthy func() bool, logger log.Logger, reg *metrics.Registry) HealthChecker {
	return &inboundHealthChecker{
		isHealthy: isHealthy,
		logger:    logger,
		reg:       reg,
	}
}
