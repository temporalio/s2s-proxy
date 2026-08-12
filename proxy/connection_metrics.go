package proxy

import (
	"context"
	"time"

	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

// connectionSampleInterval is how often a connection's session states are copied into gauges.
//
// Sampling faster does not detect a failure faster.
// A session is marked errored by its own health check.
// That check pings about once a minute.
// One minute is the floor regardless of what this is set to.
const connectionSampleInterval = 15 * time.Second

// sessionStates are pre-touched so a connection reports zero series for the states it is not in,
// rather than the state simply being absent.
// Same idiom as transport/mux/observer.go.
var sessionStates = []string{
	mux.SessionStateConnected,
	mux.SessionStateErrored,
	mux.SessionStateClosed,
}

// sampleMetrics keeps this connection's gauges current until lifetime ends.
//
// It reads mux.CountSessions, the same function describeAdmin reads.
// The metric and the admin API cannot disagree about how many sessions are up.
func (c *ClusterConnection) sampleMetrics(lifetime context.Context) {
	muxMgr, multiplexed := c.inboundMux()
	if !multiplexed {
		// A tcp connection holds no sessions.
		// Reporting zeros would be indistinguishable from a mux that has lost all of them.
		return
	}

	label := sanitizeConnectionName(c.name)
	for _, state := range sessionStates {
		metrics.ClusterConnectionMuxSessions.WithLabelValues(label, state)
	}
	metrics.ClusterConnectionMuxSessionsTarget.WithLabelValues(label).
		Set(float64(mux.DesiredMuxCount(c.remoteDefinition)))

	go func() {
		ticker := time.NewTicker(connectionSampleInterval)
		defer ticker.Stop()
		for {
			// Sampled before waiting, unlike observer.go.
			// A connection that lives less than one interval is therefore not invisible.
			c.sampleSessions(label, muxMgr)
			select {
			case <-lifetime.Done():
				metrics.ClusterConnectionMuxSessionsTarget.DeleteLabelValues(label)
				for _, state := range sessionStates {
					metrics.ClusterConnectionMuxSessions.DeleteLabelValues(label, state)
				}
				return
			case <-ticker.C:
			}
		}
	}()
}

func (c *ClusterConnection) sampleSessions(label string, muxMgr mux.MultiMuxManager) {
	counts := mux.CountSessions(muxMgr)
	set := func(state string, value int) {
		metrics.ClusterConnectionMuxSessions.WithLabelValues(label, state).Set(float64(value))
	}
	set(mux.SessionStateConnected, counts.Connected)
	set(mux.SessionStateErrored, counts.Errored)
	set(mux.SessionStateClosed, counts.Closed)
}
