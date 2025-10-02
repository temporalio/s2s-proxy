package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var (
	// This file is structured by package first, then by file.
	//So /proxy/health_check.go, /proxy/proxy.go, and then /transport/mux_connection_manager.go

	// /proxy/adminservice.go

	AdminServiceStreamsActive        = DefaultGaugeVec("admin_service_streams_active", "Number of admin service streams open", "direction")
	AdminServiceStreamDuration       = DefaultHistogramVec("admin_service_stream_duration", "The length of time each stream was open", "direction")
	AdminServiceWaitingForConnection = DefaultGaugeVec("admin_service_waiting_for_connection", "Indicates the number of requests waiting on a client", "direction")
	AdminServiceStreamsOpenedCount   = DefaultCounterVec("admin_service_streams_opened_count", "Number of streams opened", "direction")
	AdminServiceStreamsClosedCount   = DefaultCounterVec("admin_service_streams_closed_count", "Number of streams closed", "direction")
	AdminServiceStreamReqCount       = DefaultCounterVec("admin_service_stream_request_count", "Number of messages received", "direction")
	AdminServiceStreamRespCount      = DefaultCounterVec("admin_service_stream_response_count", "Number of messages received", "direction")
	// AdminServiceStreamTerminatedCount's labels are direction (inbound/outbound) and terminated_by (source/target)
	AdminServiceStreamTerminatedCount = DefaultCounterVec("admin_service_stream_terminated_count", "Stream was terminated by remote server", "direction", "terminated_by")

	// /proxy/health_check.go

	InboundIsHealthy         = DefaultGauge("health_check_success", "Inbound mux server is healthy")
	InboundHealthCheckCount  = DefaultCounter("health_check_success_count", "Inbound health check count")
	OutboundIsHealthy        = DefaultGauge("outbound_is_healthy", "Outbound proxy service is healthy")
	OutboundHealthCheckCount = DefaultCounter("outbound_health_check_count", "Outbound health check count")

	// /proxy/proxy.go

	GRPCServerMetrics     = GetStandardGRPCInterceptor("direction")
	NewProxyCount         = DefaultCounter("proxy_start_count", "Emitted once on Go process start")
	ProxyServiceCreated   = DefaultCounterVec("proxy_service_created", "Emitted once per service start", "direction")
	ProxyServiceStopped   = DefaultCounterVec("proxy_service_stopped", "Emitted on service shutdown", "direction")
	ProxyServiceRestarted = DefaultCounterVec("proxy_service_restarted", "Emitted on service shutdown", "direction")

	// /proxy/temporal_api_server.go

	GRPCServerStarted = DefaultCounterVec("grpc_server_started", "Emits when the grpc server is started", "service_name")
	GRPCServerStopped = DefaultCounterVec("grpc_server_stopped", "Emits when the grpc server is stopped", "service_name", "error")

	// /transport/grpc.go
	// Gratuitous hack: Until https://github.com/grpc-ecosystem/go-grpc-middleware/issues/783 is addressed,
	// we need to register a dependent registry with constant labels applied.

	GRPCOutboundClientMetrics = GetStandardGRPCClientInterceptor("outbound")
	GRPCInboundClientMetrics  = GetStandardGRPCClientInterceptor("inbound")

	// Mux Session

	// Every yamux session has these available, so let's use them in the prometheus tags so we can clearly see each connection
	muxSessionLabels = []string{"local_addr", "remote_addr", "mode", "config_name"}
	MuxSessionOpen   = DefaultGaugeVec("mux_connection_active", "Yes/no gauge displaying whether yamux server is connected",
		muxSessionLabels...)
	MuxStreamsActive = DefaultGaugeVec("mux_streams_active", "Immediate count of the current streams open",
		muxSessionLabels...)
	MuxObserverReportCount = DefaultCounterVec("mux_observer_report_count", "Number of observer executions",
		muxSessionLabels...)
	MuxSessionPingError = DefaultCounterVec("mux_observer_session_ping_error", "Failed ping count",
		muxSessionLabels...)
	MuxSessionPingLatency = DefaultCounterVec("mux_observer_session_ping_latency", "Ping latency for the active session",
		muxSessionLabels...)
	MuxSessionPingSuccess = DefaultCounterVec("mux_observer_session_ping_success", "Ping successes for the active session",
		muxSessionLabels...)

	// Mux Manager

	muxManagerLabels       = []string{"addr", "mode", "config_name"}
	MuxErrors              = DefaultCounterVec("mux_errors", "Number of errors observed from mux", muxManagerLabels...)
	MuxConnectionEstablish = DefaultCounterVec("mux_connection_establish", "Number of times mux has established", muxManagerLabels...)
	MuxWaitingConnections  = DefaultGaugeVec("mux_waiting_connections", "Number of goroutines waiting for a mux connection", muxManagerLabels...)
	MuxConnectionProvided  = DefaultCounterVec("mux_connection_provided", "Number of times a connection was provided from WithConnection", muxManagerLabels...)
	MuxDialFailed          = DefaultCounterVec("mux_dial_failed", "Mux failed when dialing", muxManagerLabels...)
	MuxDialSuccess         = DefaultCounterVec("mux_dial_success", "Mux succeeded on dial", muxManagerLabels...)
	MuxServerDisconnected  = DefaultCounterVec("mux_server_disconnected", "Mux server disconnected", muxManagerLabels...)

	// Translation interceptor

	translationLabels  = []string{"kind", "message_type"}
	TranslationCount   = DefaultCounterVec("translation_success", "Count of message translations", translationLabels...)
	TranslationErrors  = DefaultCounterVec("translation_error", "Count of message translation errors", translationLabels...)
	TranslationLatency = DefaultHistogramVec("translation_latency", "Latency of message translations", translationLabels...)

	UTF8RepairTranslationKind = "utf8repair"
	NamespaceTranslationKind  = "namespace"
	SearchAttrTranslationKind = "search-attribute"
	HistoryBlobMessageType    = "HistoryEventBlob"
)

func init() {
	// Deregister the existing NewGoCollector https://pkg.go.dev/github.com/prometheus/client_golang@v1.22.0/prometheus/collectors#NewGoCollector
	prometheus.Unregister(collectors.NewGoCollector())
	// Re-register the go collector with all non-debug metrics. See: https://pkg.go.dev/runtime/metrics
	prometheus.MustRegister(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
		collectors.WithoutGoCollectorRuntimeMetrics(collectors.MetricsDebug.Matcher)))
	prometheus.MustRegister(AdminServiceStreamsActive)
	prometheus.MustRegister(AdminServiceWaitingForConnection)
	prometheus.MustRegister(AdminServiceStreamDuration)
	prometheus.MustRegister(AdminServiceStreamsOpenedCount)
	prometheus.MustRegister(AdminServiceStreamsClosedCount)
	prometheus.MustRegister(AdminServiceStreamReqCount)
	prometheus.MustRegister(AdminServiceStreamRespCount)
	prometheus.MustRegister(AdminServiceStreamTerminatedCount)

	prometheus.MustRegister(InboundIsHealthy)
	prometheus.MustRegister(InboundHealthCheckCount)
	prometheus.MustRegister(OutboundIsHealthy)
	prometheus.MustRegister(OutboundHealthCheckCount)

	prometheus.MustRegister(GRPCServerMetrics)
	prometheus.MustRegister(NewProxyCount)
	prometheus.MustRegister(ProxyServiceCreated)
	prometheus.MustRegister(ProxyServiceStopped)
	prometheus.MustRegister(ProxyServiceRestarted)
	prometheus.MustRegister(GRPCServerStarted)
	prometheus.MustRegister(GRPCServerStopped)

	prometheus.MustRegister(GRPCOutboundClientMetrics)
	prometheus.MustRegister(GRPCInboundClientMetrics)

	// Mux Session
	prometheus.MustRegister(MuxSessionOpen)
	prometheus.MustRegister(MuxStreamsActive)
	prometheus.MustRegister(MuxObserverReportCount)
	prometheus.MustRegister(MuxSessionPingError)
	prometheus.MustRegister(MuxSessionPingLatency)
	prometheus.MustRegister(MuxSessionPingSuccess)

	// Mux Manager
	prometheus.MustRegister(MuxErrors)
	prometheus.MustRegister(MuxConnectionEstablish)
	prometheus.MustRegister(MuxWaitingConnections)
	prometheus.MustRegister(MuxConnectionProvided)
	prometheus.MustRegister(MuxDialFailed)
	prometheus.MustRegister(MuxDialSuccess)

	prometheus.MustRegister(TranslationCount)
	prometheus.MustRegister(TranslationErrors)
	prometheus.MustRegister(TranslationLatency)
}
