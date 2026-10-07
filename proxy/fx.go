package proxy

import (
	"go.uber.org/fx"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/outboundauth"
)

type fxParams struct {
	fx.In

	ConfigProvider      config.ConfigProvider
	LogProvider         logging.LoggerProvider
	CredentialProviders []outboundauth.Registration `group:"outbound-call-credentials"`
}

func newProxyFromFX(params fxParams) (*Proxy, error) {
	return NewProxy(
		params.ConfigProvider,
		params.LogProvider,
		WithCredentialProviders(params.CredentialProviders...),
	)
}

var Module = fx.Options(
	fx.Provide(newProxyFromFX),
)
