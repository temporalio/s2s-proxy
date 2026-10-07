package proxy

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/metrics"
)

func TestOpaqueObserverCounts(t *testing.T) {
	obs := newOpaqueObserver(log.NewTestLogger())

	passed := testutil.ToFloat64(metrics.EncryptionOpaquePassed.WithLabelValues("hsm"))
	rejected := testutil.ToFloat64(metrics.EncryptionOpaqueRejected.WithLabelValues("chasm"))

	obs.OpaquePassed("hsm", "a/StateMachineNode/Data")
	obs.OpaqueRejected("chasm", "a/ChasmNode/Data")

	require.InDelta(t, passed+1, testutil.ToFloat64(metrics.EncryptionOpaquePassed.WithLabelValues("hsm")), 0)
	require.InDelta(t, rejected+1, testutil.ToFloat64(metrics.EncryptionOpaqueRejected.WithLabelValues("chasm")), 0)
}
