package metrics

import (
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var (
	// This file is structured by package first, then by file.
	//So /proxy/health_check.go, /proxy/proxy.go, and then /transport/mux_connection_manager.go

	// /proxy/adminservice.go

	AdminServiceStreamsActive      = DefaultGaugeVec("admin_service_streams_active", "Number of admin service streams open", "direction")
	AdminServiceStreamDuration     = DefaultHistogramVec("admin_service_stream_duration", "The length of time each stream was open", "direction")
	AdminServiceStreamsOpenedCount = DefaultCounterVec("admin_service_streams_opened_count", "Number of streams opened", "direction")
	AdminServiceStreamsClosedCount = DefaultCounterVec("admin_service_streams_closed_count", "Number of streams closed", "direction")
	AdminServiceStreamReqCount     = DefaultCounterVec("admin_service_stream_request_count", "Number of messages received", "direction")
	AdminServiceStreamRespCount    = DefaultCounterVec("admin_service_stream_response_count", "Number of messages received", "direction")
	// AdminServiceStreamTerminatedCount labels are direction (inbound/outbound) and terminated_by (source/target)
	AdminServiceStreamTerminatedCount = DefaultCounterVec("admin_service_stream_terminated_count", "Stream was terminated by remote server", "direction", "terminated_by")

	// /proxy/health_check.go

	LBHealthSuccessCount = DefaultCounterVec("health_check_success", "Indicates whether the proxy reported healthy to the LB", "direction")
	LBHealthCheckCount   = DefaultCounterVec("health_check_success_count", "Emitted every health check", "direction")

	// /proxy/proxy.go

	GRPCServerMetrics = GetStandardGRPCInterceptor("direction")
	NewProxyCount     = DefaultCounter("proxy_start_count", "Emitted once on Go process start")

	// /proxy/cluster_connection.go

	GRPCServerStarted = DefaultCounterVec("grpc_server_started", "Emits when the grpc server is started", "service_name")
	GRPCServerStopped = DefaultCounterVec("grpc_server_stopped", "Emits when the grpc server is stopped", "service_name", "error")

	GRPCOutboundClientMetrics = GetStandardGRPCClientInterceptor("outbound")
	GRPCInboundClientMetrics  = GetStandardGRPCClientInterceptor("inbound")

	// /transport/mux

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
	MuxErrors              = DefaultCounterVec("mux_errors", "Number of errors observed from mux", append(muxManagerLabels, "error")...)
	MuxConnectionEstablish = DefaultCounterVec("mux_connection_establish", "Number of times mux has established", muxManagerLabels...)
	MuxDialFailed          = DefaultCounterVec("mux_dial_failed", "Mux failed when dialing", muxManagerLabels...)
	MuxDialSuccess         = DefaultCounterVec("mux_dial_success", "Mux succeeded on dial", muxManagerLabels...)
	MuxServerDisconnected  = DefaultCounterVec("mux_server_disconnected", "Mux server disconnected", muxManagerLabels...)
	NumMuxesActive         = DefaultGaugeVec("num_muxes_active", "Host-local number of active muxes for config", muxManagerLabels...)

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

// GetGRPCClientMetrics helps the GRPC client metrics objects feel more like the server one
func GetGRPCClientMetrics(directionLabel string) *grpcprom.ClientMetrics {
	switch directionLabel {
	case "outbound":
		return GRPCOutboundClientMetrics
	case "inbound":
		return GRPCInboundClientMetrics
	}
	panic("unknown direction label: " + directionLabel)
}

func init() {
	// Deregister the existing NewGoCollector https://pkg.go.dev/github.com/prometheus/client_golang@v1.22.0/prometheus/collectors#NewGoCollector
	prometheus.Unregister(collectors.NewGoCollector())
	// Re-register the go collector with all non-debug metrics. See: https://pkg.go.dev/runtime/metrics
	prometheus.MustRegister(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
		collectors.WithoutGoCollectorRuntimeMetrics(collectors.MetricsDebug.Matcher)))
	prometheus.MustRegister(AdminServiceStreamsActive)
	prometheus.MustRegister(AdminServiceStreamDuration)
	prometheus.MustRegister(AdminServiceStreamsOpenedCount)
	prometheus.MustRegister(AdminServiceStreamsClosedCount)
	prometheus.MustRegister(AdminServiceStreamReqCount)
	prometheus.MustRegister(AdminServiceStreamRespCount)
	prometheus.MustRegister(AdminServiceStreamTerminatedCount)

	prometheus.MustRegister(LBHealthSuccessCount)
	prometheus.MustRegister(LBHealthCheckCount)

	prometheus.MustRegister(GRPCServerMetrics)
	prometheus.MustRegister(NewProxyCount)
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
	prometheus.MustRegister(MuxDialFailed)
	prometheus.MustRegister(MuxDialSuccess)

	prometheus.MustRegister(TranslationCount)
	prometheus.MustRegister(TranslationErrors)
	prometheus.MustRegister(TranslationLatency)
}
