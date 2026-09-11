package vault

import (
	"cmp"
	"errors"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/crypto"

	"github.com/temporalio/s2s-proxy/metrics"
)

func TestNewCryptoMeterPrimesHandles(t *testing.T) {
	m := newCryptoMeterFor(testCollectors())

	byProvider := map[string]int{}
	for k := range m.kekDurHandle {
		byProvider[k.provider]++
	}
	require.Equal(t, map[string]int{"aws": 2, "gcp": 2, "azure": 2, "testing": 2}, byProvider)

	require.Len(t, m.kekOpHandle, 16, "4 providers x 2 operations x 2 results")
	require.Contains(t, m.kekOpHandle, kekOpKey{"aws", "wrap", "success"})
	require.Contains(t, m.kekOpHandle, kekOpKey{"testing", "unwrap", "error"})

	require.ElementsMatch(t, []dekOpKey{
		{"encrypt", "success"},
		{"encrypt", "error"},
		{"decrypt", "success"},
		{"decrypt", "error"},
	}, slices.Collect(maps.Keys(m.dekOpHandle)))
	require.Equal(t, []string{"decrypt", "encrypt"}, sortedKeys(m.dekDurHandle))
	require.Equal(t, []string{"initial", "on_demand", "scheduled"}, sortedKeys(m.dekRotationHandle))
}

func TestNewCryptoMeterUsesTheProcessCollectors(t *testing.T) {
	m := NewCryptoMeter().(*cryptoMeter)

	require.Same(t, metrics.EncryptionKEKOps, m.kekOps)
	require.Same(t, metrics.EncryptionKEKOpDur, m.kekOpDur)
	require.Same(t, metrics.EncryptionDEKCacheHits, m.cacheHits)
	require.Same(t, metrics.EncryptionDEKCacheMisses, m.cacheMisses)
	require.Same(t, metrics.EncryptionDEKCacheSize, m.cacheSize)
	require.Same(t, metrics.EncryptionDEKOps, m.dekOps)
	require.Same(t, metrics.EncryptionDEKOpDur, m.dekOpDur)
	require.Same(t, metrics.EncryptionDEKRotations, m.dekRotations)
}

func TestCryptoMeterOp(t *testing.T) {
	t.Run("a primed label set", func(t *testing.T) {
		c := testCollectors()
		newCryptoMeterFor(c).Op("aws", "wrap", "success", 0.5)

		require.Equal(t, 1.0, counter(c.kekOps, "aws", "wrap", "success"))
		requireObserved(t, c.kekOpDur.WithLabelValues("aws", "wrap"), 1, 0.5)

		// Nothing bled into the neighbouring series.
		require.Zero(t, counter(c.kekOps, "aws", "wrap", "error"))
		require.Zero(t, counter(c.kekOps, "aws", "unwrap", "success"))
		requireObserved(t, c.kekOpDur.WithLabelValues("aws", "unwrap"), 0, 0)
	})

	t.Run("an unprimed provider still records", func(t *testing.T) {
		c := testCollectors()
		newCryptoMeterFor(c).Op("vault", "wrap", "success", 0.25)

		require.Equal(t, 1.0, counter(c.kekOps, "vault", "wrap", "success"))
		requireObserved(t, c.kekOpDur.WithLabelValues("vault", "wrap"), 1, 0.25)
	})

	t.Run("an unprimed result still records", func(t *testing.T) {
		// The result is unknown but the provider and operation are not, so the
		// counter takes the fallback while the histogram uses its primed handle.
		c := testCollectors()
		newCryptoMeterFor(c).Op("aws", "wrap", "cancelled", 0.125)

		require.Equal(t, 1.0, counter(c.kekOps, "aws", "wrap", "cancelled"))
		requireObserved(t, c.kekOpDur.WithLabelValues("aws", "wrap"), 1, 0.125)
	})

	t.Run("repeated operations accumulate", func(t *testing.T) {
		c := testCollectors()
		m := newCryptoMeterFor(c)
		m.Op("gcp", "unwrap", "error", 1)
		m.Op("gcp", "unwrap", "error", 2)

		require.Equal(t, 2.0, counter(c.kekOps, "gcp", "unwrap", "error"))
		requireObserved(t, c.kekOpDur.WithLabelValues("gcp", "unwrap"), 2, 3)
	})
}

func TestCryptoMeterObserveEnvelope(t *testing.T) {
	boom := errors.New("kms unavailable")

	cases := []struct {
		name  string
		event crypto.EnvelopeEvent
		// wantOp is empty when nothing at all should be recorded.
		wantOp     string
		wantResult string
		wantSum    float64
	}{
		{
			name: "an operation with no AES step records nothing",
			// The duration is set to prove it is ignored: without CryptoAttempted
			// there is no measurement here to believe.
			event: crypto.EnvelopeEvent{Op: crypto.OpEncrypt, Err: boom, Crypto: time.Second},
		},
		{
			name:       "a successful encrypt",
			event:      crypto.EnvelopeEvent{Op: crypto.OpEncrypt, CryptoAttempted: true, Crypto: 250 * time.Microsecond},
			wantOp:     "encrypt",
			wantResult: "success",
			wantSum:    0.00025,
		},
		{
			name:       "a zero duration is still an observation",
			event:      crypto.EnvelopeEvent{Op: crypto.OpEncrypt, CryptoAttempted: true},
			wantOp:     "encrypt",
			wantResult: "success",
		},
		{
			name:       "a failed AES step",
			event:      crypto.EnvelopeEvent{Op: crypto.OpEncrypt, CryptoAttempted: true, CryptoErr: boom, Err: boom},
			wantOp:     "encrypt",
			wantResult: "error",
		},
		{
			name: "an encrypt whose wrap failed afterwards",
			// The AES step itself succeeded, so that is what this metric reports.
			// The wrap failure belongs to the KEK metrics.
			event:      crypto.EnvelopeEvent{Op: crypto.OpEncrypt, CryptoAttempted: true, Err: boom},
			wantOp:     "encrypt",
			wantResult: "success",
		},
		{
			name:       "a successful decrypt",
			event:      crypto.EnvelopeEvent{Op: crypto.OpDecrypt, CryptoAttempted: true, Crypto: time.Millisecond},
			wantOp:     "decrypt",
			wantResult: "success",
			wantSum:    0.001,
		},
		{
			name:       "an unrecognised operation is labelled rather than dropped",
			event:      crypto.EnvelopeEvent{Op: crypto.Operation(200), CryptoAttempted: true},
			wantOp:     "unknown",
			wantResult: "success",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testCollectors()
			newCryptoMeterFor(c).Observe(tc.event)
			requireOnlyDEKOp(t, c, tc.wantOp, tc.wantResult, tc.wantSum)
		})
	}
}

func TestCryptoMeterObserveCache(t *testing.T) {
	t.Run("a hit", func(t *testing.T) {
		c := testCollectors()
		newCryptoMeterFor(c).Observe(crypto.CacheEvent{Hit: true, Size: 7})

		require.Equal(t, 1.0, testutil.ToFloat64(c.cacheHits))
		require.Zero(t, testutil.ToFloat64(c.cacheMisses))
		require.Equal(t, 7.0, testutil.ToFloat64(c.cacheSize))
	})

	t.Run("a miss", func(t *testing.T) {
		c := testCollectors()
		newCryptoMeterFor(c).Observe(crypto.CacheEvent{Size: 1})

		require.Zero(t, testutil.ToFloat64(c.cacheHits))
		require.Equal(t, 1.0, testutil.ToFloat64(c.cacheMisses))
		require.Equal(t, 1.0, testutil.ToFloat64(c.cacheSize))
	})

	t.Run("the size is the latest reported and not a total", func(t *testing.T) {
		c := testCollectors()
		m := newCryptoMeterFor(c)
		m.Observe(crypto.CacheEvent{Hit: true, Size: 5})
		m.Observe(crypto.CacheEvent{Hit: true, Size: 3})

		require.Equal(t, 2.0, testutil.ToFloat64(c.cacheHits))
		require.Equal(t, 3.0, testutil.ToFloat64(c.cacheSize), "an eviction brings the gauge back down")
	})

	t.Run("an empty cache reports a zero size", func(t *testing.T) {
		c := testCollectors()
		newCryptoMeterFor(c).Observe(crypto.CacheEvent{})

		require.Zero(t, testutil.ToFloat64(c.cacheSize))
	})
}

func TestCryptoMeterObserveRotation(t *testing.T) {
	reasons := []string{"scheduled", "on_demand", "initial", "unknown"}

	cases := []struct {
		name   string
		reason crypto.RotationReason
		want   string
	}{
		{"scheduled", crypto.RotationScheduled, "scheduled"},
		{"on demand", crypto.RotationOnDemand, "on_demand"},
		{"initial", crypto.RotationInitial, "initial"},
		{"unrecognised reasons are labelled rather than dropped", crypto.RotationReason(200), "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testCollectors()
			newCryptoMeterFor(c).Observe(crypto.RotationEvent{Namespace: "ns", Reason: tc.reason})

			for _, r := range reasons {
				want := 0.0
				if r == tc.want {
					want = 1
				}

				require.Equal(t, want, counter(c.dekRotations, r), "counter for %s", r)
			}
		})
	}
}

func TestCryptoMeterObserveIgnoresPointerEvents(t *testing.T) {
	// A vault reports its events by value and the type switch matches value
	// types, so a pointer satisfies crypto.Event and then matches no case. This
	// pins that down: it is a silent drop, not a panic.
	c := testCollectors()
	newCryptoMeterFor(c).Observe(&crypto.EnvelopeEvent{Op: crypto.OpEncrypt, CryptoAttempted: true})

	requireOnlyDEKOp(t, c, "", "", 0)
}

// testCollectors mirrors the encryption collectors in [metrics], built fresh and
// left unregistered so each test observes only its own recordings. Those are
// process-wide and registered in that package's init, which makes them useless
// for asserting an exact value.
func testCollectors() cryptoCollectors {
	return cryptoCollectors{
		kekOps:       metrics.DefaultCounterVec("enc_kek_ops_total", "KEK operations", "provider", "operation", "result"),
		kekOpDur:     metrics.DefaultHistogramVec("enc_kek_op_duration_secs", "KEK operation duration", "provider", "operation"),
		cacheHits:    metrics.DefaultCounter("enc_dek_cache_hits_total", "DEK cache hits"),
		cacheMisses:  metrics.DefaultCounter("enc_dek_cache_misses_total", "DEK cache misses"),
		cacheSize:    metrics.DefaultGauge("enc_dek_cache_size", "DEK cache entries"),
		dekOps:       metrics.DefaultCounterVec("enc_dek_ops_total", "DEK operations", "operation", "result"),
		dekOpDur:     metrics.BucketedHistogramVec("enc_dek_op_duration_secs", "DEK operation duration", prometheus.ExponentialBuckets(0.00001, 4, 7), "operation"),
		dekRotations: metrics.DefaultCounterVec("enc_dek_rotations_total", "DEK rotations", "reason"),
	}
}

// sortedKeys returns the keys of m in ascending order, so a handle map can be
// compared whole rather than key by key.
func sortedKeys[K cmp.Ordered, V any](m map[K]V) []K {
	keys := slices.Collect(maps.Keys(m))
	slices.Sort(keys)

	return keys
}

// counter reads the current value of one series of vec.
func counter(vec *prometheus.CounterVec, labels ...string) float64 {
	return testutil.ToFloat64(vec.WithLabelValues(labels...))
}

// requireObserved asserts that o recorded count observations totalling sum.
// Reading a histogram takes more than [testutil.ToFloat64], which handles only
// counters and gauges.
func requireObserved(t *testing.T, o prometheus.Observer, count uint64, sum float64) {
	t.Helper()

	var m dto.Metric
	require.NoError(t, o.(prometheus.Histogram).Write(&m))
	require.Equal(t, count, m.GetHistogram().GetSampleCount())
	require.InDelta(t, sum, m.GetHistogram().GetSampleSum(), 0.000001)
}

// requireOnlyDEKOp asserts that op/result is the one DEK series recorded, with
// sum as its duration total. An empty op asserts nothing was recorded at all.
func requireOnlyDEKOp(t *testing.T, c cryptoCollectors, op, result string, sum float64) {
	t.Helper()

	for _, o := range []string{"encrypt", "decrypt", "unknown"} {
		for _, r := range []string{"success", "error"} {
			want := 0.0
			if o == op && r == result {
				want = 1
			}

			require.Equal(t, want, counter(c.dekOps, o, r), "counter for %s/%s", o, r)
		}

		var wantCount uint64
		var wantSum float64
		if o == op {
			wantCount, wantSum = 1, sum
		}

		requireObserved(t, c.dekOpDur.WithLabelValues(o), wantCount, wantSum)
	}
}
