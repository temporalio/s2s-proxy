package vault

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/metrics"
)

func TestBindingMeter(t *testing.T) {
	m := &BindingMeter{
		unbound:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: "unbound"}, []string{"namespace"}),
		mismatch: prometheus.NewCounterVec(prometheus.CounterOpts{Name: "mismatch"}, []string{"namespace"}),
	}

	m.UnboundOpen("tenant-a")
	m.UnboundOpen("tenant-a")
	m.NamespaceMismatch("tenant-b")

	require.InDelta(t, 2, testutil.ToFloat64(m.unbound.WithLabelValues("tenant-a")), 0)
	require.InDelta(t, 1, testutil.ToFloat64(m.mismatch.WithLabelValues("tenant-b")), 0)
	require.InDelta(t, 0, testutil.ToFloat64(m.mismatch.WithLabelValues("tenant-a")), 0)
}

func TestNewBindingMeterUsesTheProcessCollectors(t *testing.T) {
	m := NewBindingMeter()

	require.Same(t, metrics.EncryptionUnboundOpens, m.unbound)
	require.Same(t, metrics.EncryptionNamespaceMismatches, m.mismatch)
}
