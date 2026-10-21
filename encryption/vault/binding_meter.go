package vault

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/temporalio/s2s-proxy/metrics"
)

// BindingMeter counts the sealed payloads a strict namespace binding would
// refuse: those with no binding, and those bound to another namespace. Both are
// counted whatever the vault then does with them, so running "permissive"
// shows what switching to "strict" would break.
//
// Namespaces come from requests, so there is nothing to prime handles for, and
// the cardinality is bounded by the namespaces a connection carries.
type BindingMeter struct {
	unbound  *prometheus.CounterVec
	mismatch *prometheus.CounterVec
}

// NewBindingMeter returns a BindingMeter recording to the process-wide
// collectors.
func NewBindingMeter() *BindingMeter {
	return &BindingMeter{unbound: metrics.EncryptionUnboundOpens, mismatch: metrics.EncryptionNamespaceMismatches}
}

// UnboundOpen counts an unbound payload handed to decrypt for ns.
func (m *BindingMeter) UnboundOpen(ns string) { m.unbound.WithLabelValues(ns).Inc() }

// NamespaceMismatch counts a payload handed to decrypt for ns but bound elsewhere.
func (m *BindingMeter) NamespaceMismatch(ns string) { m.mismatch.WithLabelValues(ns).Inc() }
