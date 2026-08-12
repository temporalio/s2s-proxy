package metrics

import (
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

var (
	// This file is structured by package first, then by file.
	// So /proxy/health_check.go, /proxy/proxy.go, and then /transport/mux_connection_manager.go

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

	GRPCOutboundClientMetrics   = GetStandardGRPCClientInterceptor("outbound")
	GRPCInboundClientMetrics    = GetStandardGRPCClientInterceptor("inbound")
	GRPCIntraProxyClientMetrics = GetStandardGRPCClientInterceptor("intra_proxy")

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
	MuxServerDisconnected  = DefaultCounterVec("mux_server_disconnected", "Mux server disconnected", muxManagerLabels...)
	NumMuxesActive         = DefaultGaugeVec("num_muxes_active", "Host-local number of active muxes for config", muxManagerLabels...)

	// Connection provider
	ReceiverError    = DefaultCounterVec("receiver_error", "Number of errors observed from connection receiver", append(muxManagerLabels, "error")...)
	EstablisherError = DefaultCounterVec("establisher_error", "Number of errors observed from connection establisher", muxManagerLabels...)

	// Encryption
	EncryptionKEKOps         = DefaultCounterVec("enc_kek_ops_total", "Total KEK operations (DEK wrap/unwrap), labeled by provider, operation, and result", "provider", "operation", "result")
	EncryptionKEKOpDur       = DefaultHistogramVec("enc_kek_op_duration_secs", "Duration of KEK operations in seconds, labeled by provider and operation", "provider", "operation")
	EncryptionDEKCacheHits   = DefaultCounterVec("enc_dek_cache_hits_total", "Total DEK cache hits").WithLabelValues()
	EncryptionDEKCacheMisses = DefaultCounterVec("enc_dek_cache_misses_total", "Total DEK cache misses").WithLabelValues()
	EncryptionDEKCacheSize   = DefaultGauge("enc_dek_cache_size", "Current number of entries in the DEK cache")
	EncryptionDEKOps         = DefaultCounterVec("enc_dek_ops_total", "Total DEK operations (payload encrypt/decrypt), labeled by operation and result", "operation", "result")
	EncryptionDEKOpDur       = BucketedHistogramVec("enc_dek_op_duration_secs", "Duration of the AES-256-GCM step alone in seconds, excluding any KEK wrap or unwrap, labeled by operation", prometheus.ExponentialBuckets(0.00001, 4, 7), "operation")
	EncryptionDEKRotations   = DefaultCounterVec("enc_dek_rotations_total", "Total DEK rotations, labeled by reason", "reason")

	// Cluster connection health, sampled in /proxy/connection_metrics.go

	// config_name carries the same value as the mux metrics' config_name, sanitizeConnectionName of
	// the configured name.
	// A dashboard variable built from num_muxes_active therefore also populates these.
	//
	// This is the state breakdown nothing else exports.
	// mux_connection_active is set to 1 on every observer tick until the session's lifetime ends.
	// num_muxes_active is the size of the session map.
	// A session failing its ping but still in the map reads as healthy in both.
	ClusterConnectionMuxSessions = DefaultGaugeVec("cluster_connection_mux_sessions",
		"Mux sessions this pod holds for a cluster connection, by session state. State is refreshed by the session's own health check about once a minute, so an errored session can take that long to appear here. Not reported for a connection whose remote is plain tcp.",
		"config_name", "state")
	// The denominator that makes the state breakdown alertable.
	// A session that never established was never added to the manager.
	// The sessions held say nothing on their own about how many were meant to exist.
	ClusterConnectionMuxSessionsTarget = DefaultGaugeVec("cluster_connection_mux_sessions_target",
		"Mux sessions this pod is configured to hold for a cluster connection.",
		"config_name")
	// Version skew during a rollout.
	// collectors.NewBuildInfoCollector reports the module version.
	// This is the -X main.Version stamp the binary was actually built with.
	ProxyBuildInfo = DefaultGaugeVec("build_info",
		"Always 1, labelled with the build this process is running.", "version")

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
	case "intra_proxy":
		return GRPCIntraProxyClientMetrics
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
	prometheus.MustRegister(GRPCIntraProxyClientMetrics)

	// Mux Session
	prometheus.MustRegister(MuxSessionOpen)
	prometheus.MustRegister(MuxStreamsActive)
	prometheus.MustRegister(MuxObserverReportCount)
	prometheus.MustRegister(MuxSessionPingError)
	prometheus.MustRegister(MuxSessionPingLatency)
	prometheus.MustRegister(MuxSessionPingSuccess)

	// Mux Manager
	prometheus.MustRegister(MuxErrors)
	prometheus.MustRegister(ReceiverError)
	prometheus.MustRegister(MuxConnectionEstablish)
	prometheus.MustRegister(EstablisherError)
	prometheus.MustRegister(MuxServerDisconnected)
	prometheus.MustRegister(NumMuxesActive)

	// Encryption
	prometheus.MustRegister(

		EncryptionKEKOps,
		EncryptionKEKOpDur,
		EncryptionDEKCacheHits,
		EncryptionDEKCacheMisses,
		EncryptionDEKCacheSize,
		EncryptionDEKOps,
		EncryptionDEKOpDur,
		EncryptionDEKRotations,
	)

	prometheus.MustRegister(ClusterConnectionMuxSessions)
	prometheus.MustRegister(ClusterConnectionMuxSessionsTarget)
	prometheus.MustRegister(ProxyBuildInfo)

	prometheus.MustRegister(TranslationCount)
	prometheus.MustRegister(TranslationErrors)
	prometheus.MustRegister(TranslationLatency)
}
