package metrics

import (
	"fmt"
	"net/http"
	"regexp"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.temporal.io/server/common/log"
)

const (
	prometheusDisallowedCharacters  = `[^a-zA-Z0-9_:]`
	prometheusFirstCharacterPattern = `[^a-zA-Z_:]` // No 0-9 for first character
)

var (
	prometheusReplacePattern   = regexp.MustCompile(prometheusDisallowedCharacters)
	prometheusFirstCharPattern = regexp.MustCompile(prometheusFirstCharacterPattern)
)

// SanitizeForPrometheus cleans a string so that it may be used in prometheus namespaces, subsystems, and names
// See: https://prometheus.io/docs/concepts/data_model/
func SanitizeForPrometheus(value string) string {
	if len(value) == 0 {
		return value
	}
	if prometheusFirstCharPattern.MatchString(value[:1]) {
		value = "_" + value[1:]
	}
	return prometheusReplacePattern.ReplaceAllLiteralString(value, "_")
}

// GetStandardGRPCInterceptor returns a ServerMetrics with our preferred standard config for monitoring gRPC servers.
// Want to change/add options? Check the docs at https://pkg.go.dev/github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus@v1.1.0#section-documentation
// Some more handy links: https://prometheus.io/docs/concepts/metric_types/#histogram
func GetStandardGRPCInterceptor(labelNamesInContext ...string) *grpcprom.ServerMetrics {
	return grpcprom.NewServerMetrics(
		grpcprom.WithServerHandlingTimeHistogram(
			grpcprom.WithHistogramNamespace("temporal"),
			grpcprom.WithHistogramSubsystem("s2s_proxy"),
			// TODO: Use Native histograms or configure the buckets here?
		),
		grpcprom.WithServerCounterOptions(
			grpcprom.WithNamespace("temporal"),
			grpcprom.WithSubsystem("s2s_proxy"),
		),
		grpcprom.WithContextLabels(labelNamesInContext...),
	)
}

// DefaultGauge provides a prometheus Gauge for the requested name. The name will be sanitized, and the recommended
// namespace and subsystem will be set.
func DefaultGauge(name string, help string) prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "temporal",
		Subsystem: "s2s_proxy",
		Name:      SanitizeForPrometheus(name),
		Help:      help,
	})
}

// DefaultGaugeVec provides a prometheus GaugeVec for the requested name. The name will be sanitized, and the recommended
// namespace and subsystem will be set. Vector metrics allow the use of labels, so if you need labels on your metrics, then use this.
func DefaultGaugeVec(name string, help string, labels ...string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: "temporal",
		Subsystem: "s2s_proxy",
		Name:      SanitizeForPrometheus(name),
		Help:      help,
	}, labels)
}

// DefaultCounter provides a prometheus Counter for the requested name. The name will be sanitized, and the recommended
// namespace and subsystem will be set.
func DefaultCounter(name string, help string) prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "temporal",
		Subsystem: "s2s_proxy",
		Name:      SanitizeForPrometheus(name),
		Help:      help,
	})
}

// DefaultCounterVec provides a prometheus CounterVec for the requested name. The name will be sanitized, and the recommended
// namespace and subsystem will be set. Vector metrics allow the use of labels, so if you need labels on your metrics, then use this.
func DefaultCounterVec(name string, help string, labels ...string) *prometheus.CounterVec {
	return prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "temporal",
		Subsystem: "s2s_proxy",
		Name:      SanitizeForPrometheus(name),
		Help:      help,
	}, labels)
}

// wrapLoggerForPrometheus is necessary to adapt our temporal Logger to Prometheus's Println interface
type wrapLoggerForPrometheus struct {
	log.Logger
}

func (wls *wrapLoggerForPrometheus) Println(v ...interface{}) {
	wls.Error(fmt.Sprintln(v...))
}

// NewMetricsHandler returns an http handler that will talk to Prometheus. This uses the global-default registry right now
func NewMetricsHandler(logger log.Logger) http.Handler {
	return promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
		ErrorLog:          &wrapLoggerForPrometheus{Logger: logger},
		Registry:          nil, // use default
		EnableOpenMetrics: true,
	})
}
