package app

import (
	"go.uber.org/fx"

	"github.com/temporalio/s2s-proxy/outboundauth"
)

func WithCallCredentialsProvider(name string, factory outboundauth.Factory) fx.Option {
	return fx.Provide(
		fx.Annotate(
			func() outboundauth.Registration {
				return outboundauth.Registration{Name: name, Factory: factory}
			},
			fx.ResultTags(`group:"outbound-call-credentials"`),
		),
	)
}
