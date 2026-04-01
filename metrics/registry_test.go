package metrics_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/metrics"
)

func TestNewRegistry_TwiceInSameProcess(t *testing.T) {
	// The core regression: creating two registries must not panic.
	require.NotPanics(t, func() {
		newTestRegistry(t)
		newTestRegistry(t)
	})
}

func TestRegistry_Gatherer(t *testing.T) {
	reg := prometheus.NewRegistry()
	r, err := metrics.NewRegistry(reg, reg)
	require.NoError(t, err)
	require.Same(t, reg, r.Gatherer())
}

func TestRegistry_GRPCClientMetrics(t *testing.T) {
	r := newTestRegistry(t)

	require.Same(t, r.GRPCOutboundClientMetrics, r.GRPCClientMetrics("outbound"))
	require.Same(t, r.GRPCInboundClientMetrics, r.GRPCClientMetrics("inbound"))
	require.Same(t, r.GRPCIntraProxyClientMetrics, r.GRPCClientMetrics("intra_proxy"))
	require.Panics(t, func() { r.GRPCClientMetrics("unknown") })
}

func TestNewRegistry_Isolation(t *testing.T) {
	r1 := newTestRegistry(t)
	r2 := newTestRegistry(t)

	// Increment a counter on r1 only.
	r1.NewProxyCount.Inc()

	// Gather from both registries.
	mfs1, err := r1.Gatherer().Gather()
	require.NoError(t, err)
	mfs2, err := r2.Gatherer().Gather()
	require.NoError(t, err)

	findCounter := func(mfs []*dto.MetricFamily, name string) float64 {
		for _, mf := range mfs {
			if mf.GetName() == name {
				for _, m := range mf.GetMetric() {
					if m.Counter != nil {
						return m.Counter.GetValue()
					}
				}
			}
		}

		return 0
	}

	const counterName = "temporal_s2s_proxy_proxy_start_count"
	require.Equal(t, float64(1), findCounter(mfs1, counterName), "r1 should have count=1")
	require.Equal(t, float64(0), findCounter(mfs2, counterName), "r2 should be unaffected")
}

func newTestRegistry(t *testing.T) *metrics.Registry {
	t.Helper()

	reg := prometheus.NewRegistry()
	r, err := metrics.NewRegistry(reg, reg)
	require.NoError(t, err)
	return r
}
