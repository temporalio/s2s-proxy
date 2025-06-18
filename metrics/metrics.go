package metrics

import (
	"fmt"
	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.temporal.io/server/common/log"
	"net/http"
	"regexp"
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
func GetStandardGRPCInterceptor() *grpcprom.ServerMetrics {
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
	)
}

func DefaultGauge(name string, help string) prometheus.Gauge {
	return prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "temporal",
		Subsystem: "s2s_proxy",
		Name:      SanitizeForPrometheus(name),
		Help:      help,
	})
}
func DefaultCounter(name string, help string) prometheus.Counter {
	return prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: "temporal",
		Subsystem: "s2s_proxy",
		Name:      SanitizeForPrometheus(name),
		Help:      help,
	})
}

type wrapLoggerForPrometheus struct {
	log.Logger
}

func (wls *wrapLoggerForPrometheus) Println(v ...interface{}) {
	wls.Error(fmt.Sprintln(v...))
}

func GetMetricsHandler(logger log.Logger) http.Handler {
	return promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{
		ErrorLog:          &wrapLoggerForPrometheus{Logger: logger},
		Registry:          nil, // use default
		EnableOpenMetrics: true,
	})
}
