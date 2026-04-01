package metrics

import (
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Registry holds all Prometheus metrics for the proxy. Each instance is backed by
// an independent prometheus.Registerer so multiple instances can coexist in one process.
type Registry struct {
	// proxy/adminservice.go, proxy/admin_stream_transfer.go
	AdminServiceStreamsActive         *prometheus.GaugeVec
	AdminServiceStreamDuration        *prometheus.HistogramVec
	AdminServiceStreamsOpenedCount    *prometheus.CounterVec
	AdminServiceStreamsClosedCount    *prometheus.CounterVec
	AdminServiceStreamReqCount        *prometheus.CounterVec
	AdminServiceStreamRespCount       *prometheus.CounterVec
	AdminServiceStreamTerminatedCount *prometheus.CounterVec

	// proxy/health_check.go
	LBHealthSuccessCount *prometheus.CounterVec
	LBHealthCheckCount   *prometheus.CounterVec

	// proxy/proxy.go, proxy/cluster_connection.go
	GRPCServerMetrics           *grpcprom.ServerMetrics
	NewProxyCount               prometheus.Counter
	GRPCServerStarted           *prometheus.CounterVec
	GRPCServerStopped           *prometheus.CounterVec
	GRPCOutboundClientMetrics   *grpcprom.ClientMetrics
	GRPCInboundClientMetrics    *grpcprom.ClientMetrics
	GRPCIntraProxyClientMetrics *grpcprom.ClientMetrics

	// transport/mux/
	MuxSessionOpen         *prometheus.GaugeVec
	MuxStreamsActive       *prometheus.GaugeVec
	MuxObserverReportCount *prometheus.CounterVec
	MuxSessionPingError    *prometheus.CounterVec
	MuxSessionPingLatency  *prometheus.CounterVec
	MuxSessionPingSuccess  *prometheus.CounterVec
	MuxErrors              *prometheus.CounterVec
	MuxConnectionEstablish *prometheus.CounterVec
	MuxDialFailed          *prometheus.CounterVec
	MuxDialSuccess         *prometheus.CounterVec
	MuxServerDisconnected  *prometheus.CounterVec
	NumMuxesActive         *prometheus.GaugeVec

	// interceptor/translation_interceptor.go
	TranslationCount   *prometheus.CounterVec
	TranslationErrors  *prometheus.CounterVec
	TranslationLatency *prometheus.HistogramVec

	gatherer prometheus.Gatherer
}

// NewRegistry constructs a Registry whose metrics are all registered against reg.
// gather is stored and returned by Gatherer() for use in the /metrics HTTP handler.
func NewRegistry(reg prometheus.Registerer, gather prometheus.Gatherer) (*Registry, error) {
	muxSessionLabels := []string{"local_addr", "remote_addr", "mode", "config_name"}
	muxManagerLabels := []string{"addr", "mode", "config_name"}
	translationLabels := []string{"kind", "message_type"}

	r := &Registry{
		AdminServiceStreamsActive:         DefaultGaugeVec("admin_service_streams_active", "Number of admin service streams open", "direction"),
		AdminServiceStreamDuration:        DefaultHistogramVec("admin_service_stream_duration", "The length of time each stream was open", "direction"),
		AdminServiceStreamsOpenedCount:    DefaultCounterVec("admin_service_streams_opened_count", "Number of streams opened", "direction"),
		AdminServiceStreamsClosedCount:    DefaultCounterVec("admin_service_streams_closed_count", "Number of streams closed", "direction"),
		AdminServiceStreamReqCount:        DefaultCounterVec("admin_service_stream_request_count", "Number of messages received", "direction"),
		AdminServiceStreamRespCount:       DefaultCounterVec("admin_service_stream_response_count", "Number of messages received", "direction"),
		AdminServiceStreamTerminatedCount: DefaultCounterVec("admin_service_stream_terminated_count", "Stream was terminated by remote server", "direction", "terminated_by"),

		LBHealthSuccessCount: DefaultCounterVec("health_check_success", "Indicates whether the proxy reported healthy to the LB", "direction"),
		LBHealthCheckCount:   DefaultCounterVec("health_check_success_count", "Emitted every health check", "direction"),

		GRPCServerMetrics: GetStandardGRPCInterceptor("direction"),
		NewProxyCount:     DefaultCounter("proxy_start_count", "Emitted once on Go process start"),
		GRPCServerStarted: DefaultCounterVec("grpc_server_started", "Emits when the grpc server is started", "service_name"),
		GRPCServerStopped: DefaultCounterVec("grpc_server_stopped", "Emits when the grpc server is stopped", "service_name", "error"),

		GRPCOutboundClientMetrics:   GetStandardGRPCClientInterceptor("outbound"),
		GRPCInboundClientMetrics:    GetStandardGRPCClientInterceptor("inbound"),
		GRPCIntraProxyClientMetrics: GetStandardGRPCClientInterceptor("intra_proxy"),

		MuxSessionOpen:         DefaultGaugeVec("mux_connection_active", "Yes/no gauge displaying whether yamux server is connected", muxSessionLabels...),
		MuxStreamsActive:       DefaultGaugeVec("mux_streams_active", "Immediate count of the current streams open", muxSessionLabels...),
		MuxObserverReportCount: DefaultCounterVec("mux_observer_report_count", "Number of observer executions", muxSessionLabels...),
		MuxSessionPingError:    DefaultCounterVec("mux_observer_session_ping_error", "Failed ping count", muxSessionLabels...),
		MuxSessionPingLatency:  DefaultCounterVec("mux_observer_session_ping_latency", "Ping latency for the active session", muxSessionLabels...),
		MuxSessionPingSuccess:  DefaultCounterVec("mux_observer_session_ping_success", "Ping successes for the active session", muxSessionLabels...),

		MuxErrors:              DefaultCounterVec("mux_errors", "Number of errors observed from mux", append(muxManagerLabels, "error")...),
		MuxConnectionEstablish: DefaultCounterVec("mux_connection_establish", "Number of times mux has established", muxManagerLabels...),
		MuxDialFailed:          DefaultCounterVec("mux_dial_failed", "Mux failed when dialing", muxManagerLabels...),
		MuxDialSuccess:         DefaultCounterVec("mux_dial_success", "Mux succeeded on dial", muxManagerLabels...),
		MuxServerDisconnected:  DefaultCounterVec("mux_server_disconnected", "Mux server disconnected", muxManagerLabels...),
		NumMuxesActive:         DefaultGaugeVec("num_muxes_active", "Host-local number of active muxes for config", muxManagerLabels...),

		TranslationCount:   DefaultCounterVec("translation_success", "Count of message translations", translationLabels...),
		TranslationErrors:  DefaultCounterVec("translation_error", "Count of message translation errors", translationLabels...),
		TranslationLatency: DefaultHistogramVec("translation_latency", "Latency of message translations", translationLabels...),

		gatherer: gather,
	}

	// Register the enhanced GoCollector scoped to this registry.
	if err := reg.Register(collectors.NewGoCollector(
		collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll),
		collectors.WithoutGoCollectorRuntimeMetrics(collectors.MetricsDebug.Matcher),
	)); err != nil {
		return nil, err
	}

	toRegister := []prometheus.Collector{
		r.AdminServiceStreamsActive,
		r.AdminServiceStreamDuration,
		r.AdminServiceStreamsOpenedCount,
		r.AdminServiceStreamsClosedCount,
		r.AdminServiceStreamReqCount,
		r.AdminServiceStreamRespCount,
		r.AdminServiceStreamTerminatedCount,
		r.LBHealthSuccessCount,
		r.LBHealthCheckCount,
		r.GRPCServerMetrics,
		r.NewProxyCount,
		r.GRPCServerStarted,
		r.GRPCServerStopped,
		r.GRPCOutboundClientMetrics,
		r.GRPCInboundClientMetrics,
		r.GRPCIntraProxyClientMetrics,
		r.MuxSessionOpen,
		r.MuxStreamsActive,
		r.MuxObserverReportCount,
		r.MuxSessionPingError,
		r.MuxSessionPingLatency,
		r.MuxSessionPingSuccess,
		r.MuxErrors,
		r.MuxConnectionEstablish,
		r.MuxDialFailed,
		r.MuxDialSuccess,
		r.MuxServerDisconnected,
		r.NumMuxesActive,
		r.TranslationCount,
		r.TranslationErrors,
		r.TranslationLatency,
	}
	for _, c := range toRegister {
		if err := reg.Register(c); err != nil {
			return nil, err
		}
	}

	return r, nil
}

// Gatherer returns the prometheus.Gatherer for use with the /metrics HTTP handler.
func (r *Registry) Gatherer() prometheus.Gatherer {
	return r.gatherer
}

// GRPCClientMetrics returns the per-direction gRPC client metrics.
func (r *Registry) GRPCClientMetrics(direction string) *grpcprom.ClientMetrics {
	switch direction {
	case "outbound":
		return r.GRPCOutboundClientMetrics
	case "inbound":
		return r.GRPCInboundClientMetrics
	case "intra_proxy":
		return r.GRPCIntraProxyClientMetrics
	}
	panic("unknown direction label: " + direction)
}
