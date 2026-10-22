package proxy

import (
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
)

// visitErrorObserver counts every admin call or stream failed because a message
// could not be sealed or opened, and logs the full error behind a unary one.
// The caller of a unary call only gets a generic status, so this log line is
// the only place the cause shows up; the stream forwarder logs its own. It is
// throttled, since every call for a broken namespace or key fails the same way.
type visitErrorObserver struct {
	logger log.Logger
}

func newVisitErrorObserver(logger log.Logger) visitErrorObserver {
	return visitErrorObserver{logger: log.NewThrottledLogger(logger, func() float64 { return 1 })}
}

func (o visitErrorObserver) VisitFailed(method string, err error) {
	metrics.EncryptionAdminVisitFailures.WithLabelValues(method).Inc()
	o.logger.Error("Failing admin call: a message could not be sealed or opened", tag.Operation(method), tag.Error(err))
}

func (o visitErrorObserver) StreamVisitFailed(method string, _ error) {
	metrics.EncryptionAdminVisitFailures.WithLabelValues(method).Inc()
}
