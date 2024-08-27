package interceptor

import (
	"go.uber.org/fx"
)

var Module = fx.Provide(
	NewNamespaceTranslator,
)
