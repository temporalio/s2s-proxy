package main

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
)

func TestTokenIsReadForEachRequest(t *testing.T) {
	creds := bearerCredentialProvider{token: tokenFromEnvironment}.Get()

	t.Setenv(tokenEnvironmentVariable, "first-token")
	first, err := creds.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	t.Setenv(tokenEnvironmentVariable, "second-token")
	second, err := creds.GetRequestMetadata(t.Context())
	require.NoError(t, err)

	require.Equal(t, map[string]string{"authorization": "Bearer first-token"}, first)
	require.Equal(t, map[string]string{"authorization": "Bearer second-token"}, second)
	require.True(t, creds.RequireTransportSecurity())
}

func TestMissingTokenFailsTheCall(t *testing.T) {
	t.Setenv(tokenEnvironmentVariable, "")
	_, err := bearerCredentialProvider{token: tokenFromEnvironment}.Get().GetRequestMetadata(t.Context())
	require.ErrorContains(t, err, "bearer token is unavailable")
}

func TestProviderReplacesTheDefault(t *testing.T) {
	var provider auth.CredentialProvider
	app := fx.New(
		auth.Module,
		auth.WithCredentialProvider(bearerCredentialProvider{token: tokenFromEnvironment}),
		fx.Populate(&provider),
		fx.NopLogger,
	)
	require.NoError(t, app.Err())
	require.IsType(t, bearerCredentialProvider{}, provider)
}

func TestExampleConfiguration(t *testing.T) {
	proxyConfig, err := config.LoadConfig[config.S2SProxyConfig]("config.yaml")
	require.NoError(t, err)
	require.NoError(t, proxyConfig.Validate())
	require.Len(t, proxyConfig.ClusterConnections, 1)

	// Credentials are enabled on the local client, which must be TCP with TLS because the token requires it.
	local := proxyConfig.ClusterConnections[0].Local
	require.True(t, local.CredentialsEnabled())
	require.Equal(t, config.ConnTypeTCP, local.ConnectionType)
	require.True(t, local.TcpClient.TLSConfig.IsEnabled())
}
