package metrics

import (
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncryptionMetricsRegistered guards against declaring a metric and leaving
// it out of init: an unregistered vector still counts, so nothing else fails,
// but it never reaches /metrics, where a missing series reads as zero.
func TestEncryptionMetricsRegistered(t *testing.T) {
	collectors := map[string]prometheus.Collector{
		"EncryptionUnboundOpens":         EncryptionUnboundOpens,
		"EncryptionNamespaceMismatches":  EncryptionNamespaceMismatches,
		"EncryptionUnresolvedNamespaces": EncryptionUnresolvedNamespaces,
	}

	for name, c := range collectors {
		t.Run(name, func(t *testing.T) {
			err := prometheus.DefaultRegisterer.Register(c)
			if err == nil {
				prometheus.DefaultRegisterer.Unregister(c)
			}

			var already prometheus.AlreadyRegisteredError
			require.True(t, errors.As(err, &already), "%s is not registered in init: %v", name, err)
		})
	}
}

func TestSanitizeForPrometheus(t *testing.T) {
	assert.Equal(t, "myCoolMetric_name:123", SanitizeForPrometheus("myCoolMetric_name:123"), "Overly aggressive replacement! We mangled a valid metric.")
	assert.Equal(t, "_23myCoolMetric_name:123", SanitizeForPrometheus("123myCoolMetric_name:123"), "Metrics are not allowed to start with a number")
	assert.Equal(t, ":weirdName:123", SanitizeForPrometheus(":weirdName:123"), "Metrics ARE allowed to start with a colon!")
	assert.Equal(t, "my_Cool_Metric_Name_:456", SanitizeForPrometheus("my@Cool#Metric-Name+:456"), "Underscore and colon are the only allowed special characters")
}

func TestGetGRPCClientMetrics(t *testing.T) {
	for _, label := range []string{"outbound", "inbound", "intra_proxy", "extension"} {
		t.Run(label, func(t *testing.T) {
			require.NotNil(t, GetGRPCClientMetrics(label))
		})
	}

	t.Run("unknown label panics", func(t *testing.T) {
		require.Panics(t, func() { GetGRPCClientMetrics("nope") })
	})
}
