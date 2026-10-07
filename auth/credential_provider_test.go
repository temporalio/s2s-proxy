package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"google.golang.org/grpc/credentials"
)

type testCredentials struct{}

func (testCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer test"}, nil
}

func (testCredentials) RequireTransportSecurity() bool { return true }

type testCredentialProvider struct{}

func (testCredentialProvider) Get() credentials.PerRPCCredentials { return testCredentials{} }

func TestModuleDefaultsToEmptyCredentialProvider(t *testing.T) {
	var provider CredentialProvider
	app := fx.New(Module, fx.Populate(&provider), fx.NopLogger)
	require.NoError(t, app.Err())

	require.True(t, IsEmptyCredentialProvider(provider))
	require.Nil(t, provider.Get())
}

func TestWithCredentialProviderOverridesDefault(t *testing.T) {
	var provider CredentialProvider
	app := fx.New(Module, WithCredentialProvider(testCredentialProvider{}), fx.Populate(&provider), fx.NopLogger)
	require.NoError(t, app.Err())

	require.False(t, IsEmptyCredentialProvider(provider))
	require.Equal(t, testCredentials{}, provider.Get())
}

func TestIsEmptyCredentialProvider(t *testing.T) {
	require.True(t, IsEmptyCredentialProvider(nil))
	require.True(t, IsEmptyCredentialProvider(EmptyCredentialProvider{}))
	require.False(t, IsEmptyCredentialProvider(testCredentialProvider{}))
}
