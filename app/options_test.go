package app

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/temporalio/s2s-proxy/outboundauth"
)

func TestWithCallCredentialsProviderRegistersFactory(t *testing.T) {
	type params struct {
		fx.In
		Registrations []outboundauth.Registration `group:"outbound-call-credentials"`
	}
	var populated params
	factory := outboundauth.FactoryFunc(func(outboundauth.BuildRequest) (outboundauth.BuiltCredentials, error) {
		return outboundauth.BuiltCredentials{}, nil
	})
	application := fx.New(
		WithCallCredentialsProvider("custom", factory),
		fx.Populate(&populated),
	)
	require.NoError(t, application.Err())
	require.Len(t, populated.Registrations, 1)
	require.Equal(t, "custom", populated.Registrations[0].Name)
}
