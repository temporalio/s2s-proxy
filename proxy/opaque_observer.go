package proxy

import (
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
)

// opaqueObserver counts the opaque data met while sealing admin traffic and logs
// it, at most once a second, so a stalled replication stream or a plaintext gap
// has a reason next to it. Only kind is a metric label; path is unbounded and
// goes to the log alone.
type opaqueObserver struct {
	logger log.Logger
}

func newOpaqueObserver(logger log.Logger) opaqueObserver {
	return opaqueObserver{logger: log.NewThrottledLogger(logger, func() float64 { return 1 })}
}

func (o opaqueObserver) OpaquePassed(kind, path string) {
	metrics.EncryptionOpaquePassed.WithLabelValues(kind).Inc()
	o.logger.Warn("Sending opaque data to the peer unsealed",
		tag.NewStringTag("kind", kind), tag.NewStringTag("path", path))
}

func (o opaqueObserver) OpaqueRejected(kind, path string) {
	metrics.EncryptionOpaqueRejected.WithLabelValues(kind).Inc()
	o.logger.Error("Refusing to send opaque data that cannot be sealed; set encryption.allowOpaquePlaintext to send it unsealed",
		tag.NewStringTag("kind", kind), tag.NewStringTag("path", path))
}
