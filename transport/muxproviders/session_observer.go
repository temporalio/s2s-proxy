package muxproviders

import (
	"time"

	"github.com/hashicorp/yamux"

	"github.com/temporalio/s2s-proxy/metrics"
)

func observerLabels(localAddress string, remoteAddress string, mode string, name string) []string {
	return []string{localAddress, remoteAddress, mode, name}
}

// observeYamuxSession is a loop that pings the provided yamux session repeatedly and gathers its two
// metrics: Whether the server is alive and how many streams it has open. Intended for use as a goroutine.
func observeYamuxSession(session *yamux.Session, metricLabels []string) {
	if session == nil {
		// If we got a null session, we can't even generate tags to report
		return
	}
	var sessionActive int8 = 1
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for sessionActive == 1 {
		// Prometheus gauges are cheap, but Session.NumStreams() takes a mutex in the session! Only check once per minute
		// to minimize overhead
		select {
		case <-session.CloseChan():
			sessionActive = 0
		case <-ticker.C:
			// wake up so we can report NumStreams
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
