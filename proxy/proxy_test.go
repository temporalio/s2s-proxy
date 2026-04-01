package proxy_test

import (
	"net"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/proxy"
)

func newTestProxyRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	reg := prometheus.NewRegistry()
	r, err := metrics.NewRegistry(reg, reg)
	require.NoError(t, err)
	return r
}

// freeAddr returns a localhost address with a dynamically allocated port.
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	require.NoError(t, l.Close())
	return addr
}

// minimalProxyConfig returns the smallest valid config so NewProxy doesn't panic.
func minimalProxyConfig(t *testing.T) config.S2SProxyConfig {
	t.Helper()
	return config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{
			{
				Name: "test",
				Local: config.ClusterDefinition{
					ConnectionType: config.ConnTypeTCP,
					TcpClient:      config.TCPTLSInfo{ConnectionString: freeAddr(t)},
					TcpServer:      config.TCPTLSInfo{ConnectionString: freeAddr(t)},
				},
				Remote: config.ClusterDefinition{
					ConnectionType: config.ConnTypeTCP,
					TcpClient:      config.TCPTLSInfo{ConnectionString: freeAddr(t)},
					TcpServer:      config.TCPTLSInfo{ConnectionString: freeAddr(t)},
				},
			},
		},
	}
}

func TestNewProxy_TwoInstancesInSameProcess(t *testing.T) {
	// Regression: two Proxy instances must not panic due to duplicate metric registration.
	require.NotPanics(t, func() {
		proxy.NewProxy(
			&simpleConfigProvider{cfg: minimalProxyConfig(t)},
			logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{})),
			newTestProxyRegistry(t),
		)
		proxy.NewProxy(
			&simpleConfigProvider{cfg: minimalProxyConfig(t)},
			logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{})),
			newTestProxyRegistry(t),
		)
	})
}

type simpleConfigProvider struct {
	cfg config.S2SProxyConfig
}

func (p *simpleConfigProvider) GetS2SProxyConfig() config.S2SProxyConfig {
	return p.cfg
}
