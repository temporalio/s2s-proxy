package encryption

import (
	"iter"
	"maps"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/temporalio/temporal-proxy/pkg/crypto"

	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	// cryptoMeter records the encryption metrics. It is the [cryptoOpMeter] a
	// [KeyFactory] reports KEK operations to, and it is the [crypto.Observer] a
	// vault reports its own envelope, cache, and rotation events to. Both sides
	// land in the same place so a single meter covers a whole encryption
	// pipeline.
	//
	// Every label combination that can occur in normal operation is resolved to a
	// handle up front (see [newCryptoMeterFor]), because these metrics are
	// recorded on the request path and resolving labels per call means hashing
	// the label values and taking the vector's lock every time. A combination
	// that was not anticipated still records, just by the slower path, so a new
	// provider or a new rotation reason is never silently dropped.
	//
	// A cryptoMeter never changes after construction and the underlying
	// collectors are safe for concurrent use, so one may be shared freely. That
	// matters: [crypto.Observer] requires it.
	cryptoMeter struct {
		cryptoCollectors

		kekOpHandle       map[kekOpKey]prometheus.Counter
		kekDurHandle      map[kekDurKey]prometheus.Observer
		dekOpHandle       map[dekOpKey]prometheus.Counter
		dekDurHandle      map[string]prometheus.Observer
		dekRotationHandle map[string]prometheus.Counter
	}

	// cryptoCollectors is the set of collectors a [cryptoMeter] writes to. It
	// exists so the meter's wiring is separable from the process-wide collectors
	// in [metrics], which are registered once in that package's init and cannot
	// be reset.
	cryptoCollectors struct {
		kekOps       *prometheus.CounterVec
		kekOpDur     *prometheus.HistogramVec
		cacheHits    prometheus.Counter
		cacheMisses  prometheus.Counter
		cacheSize    prometheus.Gauge
		dekOps       *prometheus.CounterVec
		dekOpDur     *prometheus.HistogramVec
		dekRotations *prometheus.CounterVec
	}

	// kekOpKey identifies one series of the KEK operation counter.
	kekOpKey struct {
		provider  string
		operation string
		result    string
	}

	// kekDurKey identifies one series of the KEK duration histogram, which does
	// not break out the result: a failure's latency belongs with the successes it
	// is being compared against.
	kekDurKey struct {
		provider  string
		operation string
	}

	// dekOpKey identifies one series of the DEK operation counter. No provider
	// appears here because the AES step uses no KMS.
	dekOpKey struct {
		operation string
		result    string
	}
)

// NewCryptoMeter returns a cryptoMeter recording to the process-wide encryption
// collectors.
func NewCryptoMeter() CryptoMeter {
	return newCryptoMeterFor(cryptoCollectors{
		kekOps:       metrics.EncryptionKEKOps,
		kekOpDur:     metrics.EncryptionKEKOpDur,
		cacheHits:    metrics.EncryptionDEKCacheHits,
		cacheMisses:  metrics.EncryptionDEKCacheMisses,
		cacheSize:    metrics.EncryptionDEKCacheSize,
		dekOps:       metrics.EncryptionDEKOps,
		dekOpDur:     metrics.EncryptionDEKOpDur,
		dekRotations: metrics.EncryptionDEKRotations,
	})
}

// newCryptoMeterFor returns a cryptoMeter recording to c, with a handle
// resolved for every label combination the code paths below can produce: each
// provider [cryptoProviders] names crossed with the KEK operations and results,
// each DEK operation crossed with the results, and each rotation reason.
//
// Providers are deduplicated on the way in. Several schemes map onto one label,
// so the values of cryptoProviders repeat, and priming the same handle twice
// would be wasted work.
func newCryptoMeterFor(c cryptoCollectors) *cryptoMeter {
	m := &cryptoMeter{
		cryptoCollectors: cryptoCollectors{
			kekOps:       c.kekOps,
			kekOpDur:     c.kekOpDur,
			cacheHits:    c.cacheHits,
			cacheMisses:  c.cacheMisses,
			cacheSize:    c.cacheSize,
			dekOps:       c.dekOps,
			dekOpDur:     c.dekOpDur,
			dekRotations: c.dekRotations,
		},
		kekOpHandle:       make(map[kekOpKey]prometheus.Counter),
		kekDurHandle:      make(map[kekDurKey]prometheus.Observer),
		dekOpHandle:       make(map[dekOpKey]prometheus.Counter),
		dekDurHandle:      make(map[string]prometheus.Observer),
		dekRotationHandle: make(map[string]prometheus.Counter),
	}

	uniq := func(seq iter.Seq[string]) iter.Seq[string] {
		return func(yield func(string) bool) {
			seen := make(map[string]struct{})
			for v := range seq {
				if _, exists := seen[v]; exists {
					continue
				}

				seen[v] = struct{}{}
				if !yield(v) {
					return
				}
			}
		}
	}

	results := []string{"success", "error"}
	for p := range uniq(maps.Values(cryptoProviders)) {
		for _, op := range []string{"wrap", "unwrap"} {
			m.kekDurHandle[kekDurKey{p, op}] = m.kekOpDur.WithLabelValues(p, op)

			for _, r := range results {
				m.kekOpHandle[kekOpKey{p, op, r}] = m.kekOps.WithLabelValues(p, op, r)
			}
		}
	}

	for _, op := range []string{"encrypt", "decrypt"} {
		m.dekDurHandle[op] = m.dekOpDur.WithLabelValues(op)
		for _, res := range results {
			m.dekOpHandle[dekOpKey{op, res}] = m.dekOps.WithLabelValues(op, res)
		}
	}

	for _, reason := range []string{"scheduled", "on_demand", "initial"} {
		m.dekRotationHandle[reason] = m.dekRotations.WithLabelValues(reason)
	}

	return m
}

// Op records one KEK operation, counting it and observing its duration. It
// implements [cryptoOpMeter], so see that interface for what the labels mean.
func (m *cryptoMeter) Op(provider, operation, result string, seconds float64) {
	if c, ok := m.kekOpHandle[kekOpKey{provider, operation, result}]; ok {
		c.Inc()
	} else {
		m.kekOps.WithLabelValues(provider, operation, result).Inc()
	}

	if o, ok := m.kekDurHandle[kekDurKey{provider, operation}]; ok {
		o.Observe(seconds)
	} else {
		m.kekOpDur.WithLabelValues(provider, operation).Observe(seconds)
	}
}

// Observe records a vault event, implementing [crypto.Observer]. An event of a
// type this meter has no metric for is dropped rather than being an error: the
// event set belongs to the crypto package and may grow.
func (m *cryptoMeter) Observe(e crypto.Event) {
	switch ev := e.(type) {
	case crypto.EnvelopeEvent:
		m.envelopeOp(ev)
	case crypto.CacheEvent:
		m.cacheAccess(ev)
	case crypto.RotationEvent:
		m.rotated(ev)
	}
}

// envelopeOp records the AES step of one envelope operation. Note the metric is
// the AES step alone, not the operation as a whole: the KEK calls around it are
// already covered by [cryptoMeter.Op], and folding them in would bury a
// microsecond of AES under a KMS round trip.
//
// So the result label comes from CryptoErr rather than Err. A seal that
// encrypts fine and then fails to wrap its DEK counts as a successful DEK
// operation and a failed KEK one, which is what actually happened.
func (m *cryptoMeter) envelopeOp(e crypto.EnvelopeEvent) {
	// No AES step, no DEK operation to record. CryptoAttempted is the only sound
	// test for that: a zero Crypto cannot distinguish a step that never ran from
	// one that finished inside the clock's resolution, so treating a zero as
	// "never ran" would undercount both fast successes and fast failures.
	if !e.CryptoAttempted {
		return
	}

	// Every observation from here is a real measurement, including a zero, which
	// belongs in the lowest bucket rather than being withheld.
	op := e.Op.String()
	m.countDEKOp(op, resultLabel(e.CryptoErr))
	m.observeDEKDur(op, e.Crypto.Seconds())
}

// cacheAccess records a DEK cache access, and the resulting cache size along
// with it. The size is a gauge the cache never reports on its own, so an access
// is the only occasion to set it.
func (m *cryptoMeter) cacheAccess(e crypto.CacheEvent) {
	if e.Hit {
		m.cacheHits.Inc()
	} else {
		m.cacheMisses.Inc()
	}

	m.cacheSize.Set(float64(e.Size))
}

// rotated counts a DEK rotation against its reason.
func (m *cryptoMeter) rotated(e crypto.RotationEvent) {
	reason := e.Reason.String()
	if c, ok := m.dekRotationHandle[reason]; ok {
		c.Inc()
		return
	}

	m.dekRotations.WithLabelValues(reason).Inc()
}

// countDEKOp counts one DEK operation, by handle where one was primed.
func (m *cryptoMeter) countDEKOp(op, result string) {
	if c, ok := m.dekOpHandle[dekOpKey{op, result}]; ok {
		c.Inc()
		return
	}

	m.dekOps.WithLabelValues(op, result).Inc()
}

// observeDEKDur observes a DEK operation duration, by handle where one was
// primed.
func (m *cryptoMeter) observeDEKDur(op string, seconds float64) {
	if o, ok := m.dekDurHandle[op]; ok {
		o.Observe(seconds)
		return
	}

	m.dekOpDur.WithLabelValues(op).Observe(seconds)
}
