package proxy

import (
	"github.com/temporalio/s2s-proxy/interceptor"
	"go.uber.org/fx"
)

var Module = fx.Options(
	interceptor.Module,
	fx.Provide(GrpcServerOptionsProvider),
	fx.Provide(NewProxy),
)
