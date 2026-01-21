package mux

import (
	"context"
	"time"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

// registerYamuxObserverBuilder makes a closure with the muxCategory and logger so that sessions belonging to the same
// grpcMuxManager all emit the same metrics together
func registerYamuxObserverBuilder(muxCategory string, logger log.Logger) session.StartManagedComponentFn {
	return func(lifetime context.Context, id string, session *yamux.Session) {
		go emitYamuxMetrics(lifetime, muxCategory, id, session, logger)
	}
}

// emitYamuxMetrics creates a loop that pings the provided yamux session repeatedly and gathers its two
// metrics: Whether the server is alive and how many streams it has open. Intended for use as a goroutine.
func emitYamuxMetrics(lifetime context.Context, muxCategory string, id string, session *yamux.Session, logger log.Logger) {
	logger.Info("mux session watcher starting", tag.NewStringTag("remote_addr", session.RemoteAddr().String()),
		tag.NewStringTag("local_addr", session.LocalAddr().String()),
		tag.NewStringTag("mux_id", id))
	metricLabels := []string{session.LocalAddr().String(), session.RemoteAddr().String(), "muxed", muxCategory}
	if session == nil {
		// If we got a null session, we can't even generate tags to report
		return
	}
	metrics.MuxSessionPingError.WithLabelValues(metricLabels...)
	metrics.MuxSessionPingLatency.WithLabelValues(metricLabels...)
	metrics.MuxSessionPingSuccess.WithLabelValues(metricLabels...)
	var sessionActive int8 = 1
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for sessionActive == 1 {
		// Prometheus gauges are cheap, but Session.NumStreams() takes a mutex in the session! Only check once per minute
		// to minimize overhead
		select {
		case <-lifetime.Done():
			sessionActive = 0
		case <-ticker.C:
			// wake up so we can report NumStreams
		}
		dur, err := session.Ping()
		if err != nil {
			metrics.MuxSessionPingError.WithLabelValues(metricLabels...).Inc()
		} else {
			metrics.MuxSessionPingLatency.WithLabelValues(metricLabels...).Add(float64(dur))
			metrics.MuxSessionPingSuccess.WithLabelValues(metricLabels...).Inc()
		}
		metrics.MuxSessionOpen.WithLabelValues(metricLabels...).Set(float64(sessionActive))
		if sessionActive == 1 {
			metrics.MuxStreamsActive.WithLabelValues(metricLabels...).Set(float64(session.NumStreams()))
		} else {
			// Clean up the label so we don't report it forever
			metrics.MuxStreamsActive.DeleteLabelValues(metricLabels...)
			metrics.MuxSessionOpen.DeleteLabelValues(metricLabels...)
		}
		metrics.MuxObserverReportCount.WithLabelValues(metricLabels...).Inc()
	}
}
