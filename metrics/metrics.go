package metrics

import (
	"fmt"
	"time"

	"github.com/temporalio/s2s-proxy/config"

	"github.com/uber-go/tally/v4"
	"github.com/uber-go/tally/v4/prometheus"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

const (
	PROXY_START_COUNT  = "proxy_start_count"
	HEALTH_CHECK_COUNT = "health_check_count"
)

// tally sanitizer options that satisfy both Prometheus and M3 restrictions.
// This will rename metrics at the tally emission level, so metrics name we
// use maybe different from what gets emitted. In the current implementation
// it will replace - and . with _
// We should still ensure that the base metrics are prometheus compatible,
// but this is necessary as the same prom client initialization is used by
// our system workflows.
var (
	safeCharacters      = []rune{'_'}
	safeValueCharacters = []rune{'_', '-', '.'}

	sanitizeOptions = tally.SanitizeOptions{
		NameCharacters: tally.ValidCharacters{
			Ranges:     tally.AlphanumericRange,
			Characters: safeCharacters,
		},
		KeyCharacters: tally.ValidCharacters{
			Ranges:     tally.AlphanumericRange,
			Characters: safeCharacters,
		},
		ValueCharacters: tally.ValidCharacters{
			Ranges:     tally.AlphanumericRange,
			Characters: safeValueCharacters,
		},
		ReplacementCharacter: tally.DefaultReplacementCharacter,
	}

	defaultHistogramBuckets = []time.Duration{
		0 * time.Millisecond,
		1 * time.Millisecond,
		2 * time.Millisecond,
		5 * time.Millisecond,
		10 * time.Millisecond,
		25 * time.Millisecond,
		50 * time.Millisecond,
		75 * time.Millisecond,
		100 * time.Millisecond,
		200 * time.Millisecond,
		300 * time.Millisecond,
		400 * time.Millisecond,
		500 * time.Millisecond,
		600 * time.Millisecond,
		800 * time.Millisecond,
		1 * time.Second,
		2 * time.Second,
		3 * time.Second,
		4 * time.Second,
		5 * time.Second,
		7 * time.Second,
		10 * time.Second,
		15 * time.Second,
		20 * time.Second,
		30 * time.Second,
		45 * time.Second,
		60 * time.Second,
		120 * time.Second,
		180 * time.Second,
		240 * time.Second,
		300 * time.Second,
		450 * time.Second,
		600 * time.Second,
		900 * time.Second,
		1200 * time.Second,
		1800 * time.Second,
		2400 * time.Second,
		3000 * time.Second,
		3600 * time.Second,
	}
)

func newMetricsScope(
	configProvider config.ConfigProvider,
	logger log.Logger,
) (tally.Scope, error) {
	scope := tally.NoopScope
	s2sConfig := configProvider.GetS2SProxyConfig()
	if metricsCfg := s2sConfig.Metrics; metricsCfg != nil {
		reporterConfig := prometheus.ConfigurationOptions{
			OnError: func(err error) {
				logger.Warn("error in prometheus reporter", tag.Error(err))
			},
		}

		prometheusConfig := getPrometheusConfig(metricsCfg.Prometheus, logger)
		if prometheusConfig != nil {
			reporter, err := prometheusConfig.NewReporter(reporterConfig)
			if err != nil {
				logger.Error("error creating prometheus reporter", tag.Error(err))
				return nil, err
			}
			scopeOpts := tally.ScopeOptions{
				Tags: map[string]string{
					"service_name":          "s2s-proxy",
					"temporal_service_type": "s2s-proxy",
				},
				CachedReporter:  reporter,
				Separator:       prometheus.DefaultSeparator,
				SanitizeOptions: &sanitizeOptions,
			}

			scope, _ = tally.NewRootScope(scopeOpts, time.Second)
		}
	}

	return scope, nil
}

func getPrometheusConfig(config config.PrometheusConfig, logger log.Logger) *prometheus.Configuration {
	if config.ListenAddress == "" {
		logger.Warn("no prometheus host-port supplied. prometheus reporter will not be configured")
		return nil
	}

	if config.Framework != "tally" {
		logger.Warn(fmt.Sprintf("prometheus framework %s is not supported. prometheus reporter will not be configured", config.Framework))
		return nil
	}

	return &prometheus.Configuration{
		ListenAddress:           config.ListenAddress,
		TimerType:               "histogram",
		DefaultHistogramBuckets: generateHistogramBuckets(),
	}
}

func generateHistogramBuckets() []prometheus.HistogramObjective {
	var result []prometheus.HistogramObjective
	for _, b := range defaultHistogramBuckets {
		result = append(result, prometheus.HistogramObjective{
			Upper: b.Seconds(),
		})
	}

	return result
}
