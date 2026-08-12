package proxy

import (
	"maps"
	"slices"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/mux"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

// fakeMuxManager reports a fixed set of sessions.
// Only GetMuxConnections is reached by mux.CountSessions.
// The rest of MultiMuxManager is here to satisfy the interface.
type fakeMuxManager struct {
	mux.MultiMuxManager
	sessions map[string]session.ManagedMuxSession
}

func (f fakeMuxManager) GetMuxConnections() map[string]session.ManagedMuxSession { return f.sessions }

// fakeSession reports one state and nothing else.
type fakeSession struct {
	session.ManagedMuxSession
	info *session.MuxSessionInfo
}

func (f fakeSession) State() *session.MuxSessionInfo { return f.info }

func sessionsInStates(states ...session.MuxSessionState) map[string]session.ManagedMuxSession {
	out := map[string]session.ManagedMuxSession{}
	for i, st := range states {
		out[string(rune('a'+i))] = fakeSession{info: &session.MuxSessionInfo{State: st}}
	}
	return out
}

// The sampler reads mux.CountSessions, the same function the admin API reads.
// A divergence between the metric and the endpoint would have to be a bug in one of the two callers.
func TestSampleSessionsReportsEveryState(t *testing.T) {
	cases := []struct {
		name     string
		sessions map[string]session.ManagedMuxSession
		want     map[string]float64
	}{
		{
			name:     "all connected",
			sessions: sessionsInStates(session.Connected, session.Connected),
			want:     map[string]float64{"connected": 2, "error": 0, "closed": 0},
		},
		{
			// The case no existing metric can express.
			// mux_connection_active reads 1 and num_muxes_active counts the session.
			// Both report this as healthy.
			name:     "one of three failing its ping",
			sessions: sessionsInStates(session.Connected, session.Connected, session.Error),
			want:     map[string]float64{"connected": 2, "error": 1, "closed": 0},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			label := "test_" + t.Name()
			t.Cleanup(func() {
				for _, state := range sessionStates {
					metrics.ClusterConnectionMuxSessions.DeleteLabelValues(label, state)
				}
			})

			cc := &ClusterConnection{}
			cc.sampleSessions(label, fakeMuxManager{sessions: c.sessions})

			// Every pre-touched label has to be one the sampler writes.
			// A label created at zero and never written is indistinguishable from a state that is genuinely empty.
			require.ElementsMatch(t, sessionStates, slices.Collect(maps.Keys(c.want)))

			for state, want := range c.want {
				require.Equal(t, want, gaugeValue(t, metrics.ClusterConnectionMuxSessions.WithLabelValues(label, state)),
					"state %q", state)
			}
		})
	}
}

// The target is the denominator that makes the state breakdown alertable.
// Sessions held says nothing on its own about how many were meant to exist.
func TestMuxSessionTargetUsesTheConfiguredCount(t *testing.T) {
	a := getDynamicPlccAddresses(t)
	withCount := makeMuxClusterConfig("counted", config.ConnTypeMuxServer, localFVI, remoteFVI,
		a.localProxyOutbound, a.localTemporalAddr, a.localProxyOutbound, a.localProxyInbound,
		func(cc *config.ClusterConnConfig) { cc.Remote.MuxCount = 7 })

	require.Equal(t, 7, mux.DesiredMuxCount(withCount.Remote))

	// An unset count is the historical default rather than zero.
	// Otherwise every existing config would report a target of none.
	withCount.Remote.MuxCount = 0
	require.Equal(t, 10, mux.DesiredMuxCount(withCount.Remote))
}

// gaugeValue reads a gauge without pulling in prometheus/testutil.
// That would add a dependency for one assertion.
func gaugeValue(t *testing.T, g prometheus.Gauge) float64 {
	t.Helper()
	var m dto.Metric
	require.NoError(t, g.Write(&m))
	return m.GetGauge().GetValue()
}
