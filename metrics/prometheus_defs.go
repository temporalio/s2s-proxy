package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var (
	// This file is structured by package first, then by file.
	//So /proxy/health_check.go, /proxy/proxy.go, and then /transport/mux_connection_manager.go

	// /proxy/adminservice.go

	AdminServiceStreamsActive = DefaultGaugeVec("admin_service_streams_active", "Number of admin service streams open",
		"direction")

	// /proxy/health_check.go

	HealthCheckIsHealthy    = DefaultGauge("health_check_success", "s2s-proxy service is healthy")
	HealthCheckHealthyCount = DefaultCounter("health_check_success_count", "Number of healthy checks from s2s-proxy since service start")

	// /proxy/proxy.go

	GRPCServerMetrics = GetStandardGRPCInterceptor("direction")
	ProxyStartCount   = DefaultCounter("proxy_start_count", "Emitted once per startup")

	// /transport/grpc.go
	// Gratuitous hack: Until https://github.com/grpc-ecosystem/go-grpc-middleware/issues/783 is addressed,
	// we need to register a dependent registry with constant labels applied.

	GRPCOutboundClientMetrics = GetStandardGRPCClientInterceptor("outbound")
	GRPCInboundClientMetrics  = GetStandardGRPCClientInterceptor("inbound")

	// /transport/mux_connection_manager.go

	// Every yamux session has these available, so let's use them in the prometheus tags so we can clearly see each connection
	muxSessionLabels = []string{"local_addr", "remote_addr", "mode", "config_name"}
	MuxSessionOpen   = DefaultGaugeVec("mux_connection_active", "Yes/no gauge displaying whether yamux server is connected",
		muxSessionLabels...)
	MuxStreamsActive = DefaultGaugeVec("mux_streams_active", "Immediate count of the current streams open",
		muxSessionLabels...)
	MuxObserverReportCount = DefaultCounterVec("mux_observer_report_count", "Number of observer executions",
		muxSessionLabels...)
)

func init() {
	// Deregister the existing NewGoCollector https://pkg.go.dev/github.com/prometheus/client_golang@v1.22.0/prometheus/collectors#NewGoCollector
	prometheus.Unregister(collectors.NewGoCollector())
	// Re-register the go collector with all non-debug metrics. See: https://pkg.go.dev/runtime/metrics
	prometheus.MustRegister(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
		collectors.WithoutGoCollectorRuntimeMetrics(collectors.MetricsDebug.Matcher)))
	prometheus.MustRegister(ProxyStartCount)
	prometheus.MustRegister(GRPCServerMetrics)
	prometheus.MustRegister(GRPCOutboundClientMetrics)
	prometheus.MustRegister(GRPCInboundClientMetrics)
	prometheus.MustRegister(HealthCheckIsHealthy)
	prometheus.MustRegister(HealthCheckHealthyCount)
	prometheus.MustRegister(AdminServiceStreamsActive)
	prometheus.MustRegister(MuxSessionOpen)
	prometheus.MustRegister(MuxStreamsActive)
	prometheus.MustRegister(MuxObserverReportCount)
}
