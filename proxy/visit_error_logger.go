package proxy

import (
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

// visitErrorLogger logs the full error behind every unary admin call failed
// because a message could not be sealed or opened. The caller only gets a
// generic status, so this log line is the only place the cause shows up. It is
// throttled, since every call for a broken namespace or key fails the same way.
type visitErrorLogger struct {
	logger log.Logger
}

func newVisitErrorLogger(logger log.Logger) visitErrorLogger {
	return visitErrorLogger{logger: log.NewThrottledLogger(logger, func() float64 { return 1 })}
}

func (l visitErrorLogger) VisitFailed(method string, err error) {
	l.logger.Error("Failing admin call: a message could not be sealed or opened", tag.Operation(method), tag.Error(err))
}
