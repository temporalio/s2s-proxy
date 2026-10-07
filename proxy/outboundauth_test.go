package proxy

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc/credentials"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/outboundauth"
)

type proxyTestCredentials struct{}

func (proxyTestCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"x-custom-auth": "value"}, nil
}

func (proxyTestCredentials) RequireTransportSecurity() bool {
	return true
}

func TestNewProxyBuildsDestinationCredentials(t *testing.T) {
	proxyConfig := config.S2SProxyConfig{ClusterConnections: []config.ClusterConnConfig{{
		Name: "migration",
		Local: config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpClient: config.TCPTLSInfo{
				ConnectionString: "frontend.example.invalid:7233",
				TLSConfig: encryption.TLSConfig{
					CAServerName: "frontend.example.invalid",
				},
			},
			TcpServer: config.TCPTLSInfo{ConnectionString: "localhost:0"},
			CallCredentials: &config.CallCredentialsConfig{
				Provider:   "custom",
				Properties: map[string]string{"audience": "frontend"},
			},
		},
		Remote: config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpClient:      config.TCPTLSInfo{ConnectionString: "remote.example.invalid:7233"},
			TcpServer:      config.TCPTLSInfo{ConnectionString: "localhost:0"},
		},
	}}}
	configProvider := config.NewMockConfigProvider(proxyConfig)
	loggerProvider := logging.NewLoggerProvider(log.NewNoopLogger(), configProvider)
	var calls atomic.Int32
	var received outboundauth.BuildRequest

	proxy, err := NewProxy(
		configProvider,
		loggerProvider,
		WithCredentialProviders(outboundauth.Registration{
			Name: "custom",
			Factory: outboundauth.FactoryFunc(func(request outboundauth.BuildRequest) (outboundauth.BuiltCredentials, error) {
				calls.Add(1)
				received = request
				return outboundauth.BuiltCredentials{
					PerRPC:            proxyTestCredentials{},
					OwnedMetadataKeys: []string{"x-custom-auth"},
				}, nil
			}),
		}),
	)
	require.NoError(t, err)
	t.Cleanup(proxy.Stop)
	assert.Equal(t, int32(1), calls.Load())
	assert.Equal(t, outboundauth.Target{
		ClusterConnection: "migration",
		Destination:       outboundauth.DestinationLocal,
		Address:           "frontend.example.invalid:7233",
		Transport:         "tcp",
	}, received.Target)
	assert.Equal(t, "frontend", received.Properties["audience"])
}

func TestNewProxyRejectsCredentialProviderConfiguration(t *testing.T) {
	loggerProvider := logging.NewLoggerProvider(log.NewNoopLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))

	t.Run("duplicate registration", func(t *testing.T) {
		registration := outboundauth.Registration{Name: "custom", Factory: outboundauth.FactoryFunc(func(outboundauth.BuildRequest) (outboundauth.BuiltCredentials, error) {
			return outboundauth.BuiltCredentials{}, nil
		})}
		_, err := NewProxy(
			config.NewMockConfigProvider(config.S2SProxyConfig{}),
			loggerProvider,
			WithCredentialProviders(registration, registration),
		)
		require.ErrorContains(t, err, "duplicate")
	})

	t.Run("unknown provider", func(t *testing.T) {
		proxyConfig := config.S2SProxyConfig{ClusterConnections: []config.ClusterConnConfig{{
			Name: "migration",
			Local: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpClient: config.TCPTLSInfo{
					ConnectionString: "frontend.example.invalid:7233",
					TLSConfig:        encryption.TLSConfig{CAServerName: "frontend.example.invalid"},
				},
				CallCredentials: &config.CallCredentialsConfig{Provider: "missing"},
			},
			Remote: config.ClusterDefinition{ConnectionType: config.ConnTypeMuxServer},
		}}}
		configProvider := config.NewMockConfigProvider(proxyConfig)
		_, err := NewProxy(configProvider, logging.NewLoggerProvider(log.NewNoopLogger(), configProvider))
		require.ErrorContains(t, err, "unknown outbound call credentials provider")
	})
}

var _ credentials.PerRPCCredentials = proxyTestCredentials{}
