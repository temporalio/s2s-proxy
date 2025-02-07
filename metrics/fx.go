package metrics

import (
	"go.uber.org/fx"
)

var Module = fx.Provide(
	newMetricsScope,
)
