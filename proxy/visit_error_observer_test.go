package proxy

import (
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
)

// errorRecorder keeps what is logged at Error. A nil embedded Logger means any
// other level panics rather than being quietly dropped.
type errorRecorder struct {
	log.Logger
	msgs []string
	tags [][]tag.Tag
}

func (r *errorRecorder) Error(msg string, tags ...tag.Tag) {
	r.msgs = append(r.msgs, msg)
	r.tags = append(r.tags, tags)
}

// The caller of a unary admin call failed this way only gets a generic status,
// so the full error has to reach the log.
func TestVisitErrorObserverLogsTheFullError(t *testing.T) {
	rec := &errorRecorder{}
	newVisitErrorObserver(rec).VisitFailed("/AdminService/GetDLQReplicationMessages", errors.New(`failed to resolve namespace ID "id-a"`))

	require.Len(t, rec.msgs, 1)
	values := map[string]any{}
	for _, tg := range rec.tags[0] {
		values[tg.Key()] = tg.Value()
	}
	require.Equal(t, "/AdminService/GetDLQReplicationMessages", values["operation"])
	require.Contains(t, fmt.Sprint(values["error"]), "id-a")
}

// Unary calls and streams both count, so there is one series to alert on, but
// only unary failures are logged here: the stream forwarder already logs the
// full error of a stream it relays.
func TestVisitErrorObserverCountsUnaryAndStreams(t *testing.T) {
	const (
		unary  = "/AdminService/GetDLQReplicationMessages"
		stream = "/AdminService/StreamWorkflowReplicationMessages"
	)
	rec := &errorRecorder{}
	obs := newVisitErrorObserver(rec)

	unaryBefore := testutil.ToFloat64(metrics.EncryptionAdminVisitFailures.WithLabelValues(unary))
	streamBefore := testutil.ToFloat64(metrics.EncryptionAdminVisitFailures.WithLabelValues(stream))

	obs.VisitFailed(unary, errors.New("kms down"))
	obs.StreamVisitFailed(stream, errors.New("kms down"))

	require.InDelta(t, unaryBefore+1, testutil.ToFloat64(metrics.EncryptionAdminVisitFailures.WithLabelValues(unary)), 0)
	require.InDelta(t, streamBefore+1, testutil.ToFloat64(metrics.EncryptionAdminVisitFailures.WithLabelValues(stream)), 0)
	require.Len(t, rec.msgs, 1, "only the unary failure is logged")
}
