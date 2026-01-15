package logging

import "go.uber.org/fx"

var Module = fx.Provide(
	NewLoggerProvider,
)
