package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/outboundauth"
)

func TestBearerFactoryReadsEnvironmentForEachRequest(t *testing.T) {
	t.Setenv(tokenEnvironmentVariable, "first-token")
	built, err := bearerFactory().Build(outboundauth.BuildRequest{})
	require.NoError(t, err)
	assert.Equal(t, []string{"authorization"}, built.OwnedMetadataKeys)

	first, err := built.PerRPC.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	t.Setenv(tokenEnvironmentVariable, "second-token")
	second, err := built.PerRPC.GetRequestMetadata(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "Bearer first-token", first["authorization"])
	assert.Equal(t, "Bearer second-token", second["authorization"])
}

func TestBearerFactoryFailsWithoutEnvironmentToken(t *testing.T) {
	t.Setenv(tokenEnvironmentVariable, "")
	built, err := bearerFactory().Build(outboundauth.BuildRequest{})
	require.NoError(t, err)
	_, err = built.PerRPC.GetRequestMetadata(t.Context())
	require.ErrorContains(t, err, "unavailable")
}

func TestExampleConfiguration(t *testing.T) {
	proxyConfig, err := config.LoadConfig[config.S2SProxyConfig]("config.yaml")
	require.NoError(t, err)
	require.NoError(t, proxyConfig.Validate())
	require.Len(t, proxyConfig.ClusterConnections, 1)
	callCredentials := proxyConfig.ClusterConnections[0].Local.CallCredentials
	require.NotNil(t, callCredentials)
	assert.Equal(t, "example-bearer", callCredentials.Provider)
}
