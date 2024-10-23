package transport

import (
	"go.uber.org/fx"
)

var Module = fx.Provide(
	NewTransportManager,
)
