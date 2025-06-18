package metrics

import "github.com/prometheus/client_golang/prometheus"

var (
	// Proxy-server metrics

	ProxyStartCount   = DefaultCounter("proxy_start_count", "Emitted once per startup")
	GRPCServerMetrics = GetStandardGRPCInterceptor()

	// Health Check metrics

	HealthCheckIsHealthy    = DefaultGauge("health_check_success", "s2s-proxy service is healthy")
	HealthCheckHealthyCount = DefaultCounter("health_check_success_count", "Number of healthy checks from s2s-proxy since service start")
)

func init() {
	prometheus.MustRegister(ProxyStartCount)
	prometheus.MustRegister(GRPCServerMetrics)
	prometheus.MustRegister(HealthCheckIsHealthy)
	prometheus.MustRegister(HealthCheckHealthyCount)
}
