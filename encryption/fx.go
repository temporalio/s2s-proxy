package encryption

import "go.uber.org/fx"

var Module = fx.Provide(
	NewTLSConfigProfilder,
)
