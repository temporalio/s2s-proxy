package mux

import (
	"time"

	"github.com/hashicorp/yamux"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

// observerLabels is a convenience function that gives names to the string array supplied to the yamux observer
func observerLabels(localAddress string, remoteAddress string, mode string, name string) []string {
	return []string{localAddress, remoteAddress, mode, name}
}

// registerYamuxObserver creates a loop that pings the provided yamux session repeatedly and gathers its two
// metrics: Whether the server is alive and how many streams it has open. Intended for use as a goroutine.
func registerYamuxObserver(muxCategory string, logger log.Logger) session.StartManagedComponentFn {
	return func(id string, session *yamux.Session, shutdown channel.ShutdownOnce) {
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
			case <-shutdown.Channel():
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
			}
			metrics.MuxObserverReportCount.WithLabelValues(metricLabels...).Inc()
		}
	}
}
