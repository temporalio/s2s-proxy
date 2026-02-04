package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	releaseutil "helm.sh/helm/v4/pkg/release/v1/util"
)

type Tuple[K, V any] struct {
	k K
	v V
}

func NewTuple[K, V any](k K, v V) Tuple[K, V] {
	return Tuple[K, V]{k: k, v: v}
}

func TestLoadS2SConfig(t *testing.T) {
	samplePath := filepath.Join("..", "develop", "legacyconfig", "empty-config.yaml")

	s2sConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	assert.NoError(t, err)
	assert.Equal(t, "inbound-proxy", s2sConfig.Inbound.Name)
	assert.Equal(t, "outbound-proxy", s2sConfig.Outbound.Name)
	assert.Equal(t, TCPTransport, s2sConfig.Inbound.Client.Type)
	assert.Equal(t, TCPTransport, s2sConfig.Inbound.Server.Type)
	assert.Equal(t, "outbound-proxy", s2sConfig.Outbound.Name)
	assert.Equal(t, []NameMappingConfig{
		{
			LocalName:  "example",
			RemoteName: "example.cloud",
		},
	}, s2sConfig.NamespaceNameTranslation.Mappings)

	aclConfig := s2sConfig.Inbound.ACLPolicy
	assert.NotEmpty(t, aclConfig)
	assert.Greater(t, len(aclConfig.AllowedMethods.AdminService), 0)
	assert.Equal(t, []string{"namespace1", "namespace2"}, aclConfig.AllowedNamespaces)
	assert.Equal(t, HTTP, s2sConfig.HealthCheck.Protocol)
	assert.Equal(t, int64(100), *s2sConfig.Inbound.APIOverrides.AdminService.DescribeCluster.Response.FailoverVersionIncrement)
}

func TestLoadS2SConfigMux(t *testing.T) {
	configFiles := []string{
		"cluster-a-mux-client-proxy.yaml",
		"cluster-b-mux-server-proxy.yaml",
	}

	for _, file := range configFiles {
		samplePath := filepath.Join("..", "develop", "config", file)
		s2sConfig, err := LoadConfig[S2SProxyConfig](samplePath)
		assert.Equal(t, MuxTransport, s2sConfig.Inbound.Server.Type)
		assert.Equal(t, MuxTransport, s2sConfig.Outbound.Client.Type)
		assert.NoError(t, err)
	}
}

func TestBasic(t *testing.T) {
	samplePath := filepath.Join("..", "develop", "config", "sample-cluster-conn-config.yaml")

	proxyConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	require.NoError(t, err)
	require.Equal(t, 1, len(proxyConfig.ClusterConnections))
	require.Equal(t, "127.0.0.1:911", proxyConfig.ClusterConnections[0].RemoteClusterHealthCheck.ListenAddress)
	require.Equal(t, "127.0.0.1:912", proxyConfig.ClusterConnections[0].LocalClusterHealthCheck.ListenAddress)
	require.Equal(t, "myCoolCluster", proxyConfig.ClusterConnections[0].Name)
	require.Equal(t, ConnectionType("mux-server"), proxyConfig.ClusterConnections[0].Remote.ConnectionType)
	require.Equal(t, 10, proxyConfig.ClusterConnections[0].Remote.MuxCount)
	require.Equal(t, "127.0.0.1:9004", proxyConfig.ClusterConnections[0].Remote.MuxAddressInfo.ConnectionString)
	require.Equal(t, "", proxyConfig.ClusterConnections[0].Remote.TcpServer.ConnectionString)
	require.Equal(t, "", proxyConfig.ClusterConnections[0].Remote.TcpClient.ConnectionString)
	require.True(t, proxyConfig.ClusterConnections[0].Remote.MuxAddressInfo.TLSConfig.SkipCAVerification)
	nsTranslation, err := proxyConfig.ClusterConnections[0].NamespaceTranslation.AsLocalToRemoteBiMap()
	require.NoError(t, err)
	require.Equal(t, "remoteName", nsTranslation.Get("localName"))
	require.Equal(t, "localName", nsTranslation.Inverse().Get("remoteName"))
	require.Equal(t, "", nsTranslation.Get("UnknownName"))
	require.Equal(t, "", nsTranslation.Inverse().Get("UnknownName"))
	require.Equal(t, NewTuple("", false), NewTuple(nsTranslation.GetExists("UnknownName")))
	require.Equal(t, NewTuple("", false), NewTuple(nsTranslation.Inverse().GetExists("UnknownName")))
	saTranslation, err := proxyConfig.ClusterConnections[0].SearchAttributeTranslation.AsLocalToRemoteSATranslation()
	require.NoError(t, err)
	require.Equal(t, "remoteSearchAttribute", saTranslation.Get("namespace-id-1", "localSearchAttribute"))
	require.Equal(t, "localSearchAttribute", saTranslation.Inverse().Get("namespace-id-1", "remoteSearchAttribute"))
}

func TestConversion(t *testing.T) {
	samplePath := filepath.Join("..", "develop", "legacyconfig", "empty-config.yaml")

	proxyConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	require.NoError(t, err)
	converted := ToClusterConnConfig(proxyConfig)
	require.Equal(t, 1, len(converted.ClusterConnections))
	require.Nil(t, converted.Inbound)
	require.Nil(t, converted.Outbound)
	require.Equal(t, ConnTypeTCP, converted.ClusterConnections[0].Remote.ConnectionType)
	require.False(t, converted.ClusterConnections[0].Remote.TcpServer.TLSConfig.SkipCAVerification)
	require.Equal(t, ConnTypeTCP, converted.ClusterConnections[0].Local.ConnectionType)
	require.Equal(t, "AddOrUpdateRemoteCluster", converted.ClusterConnections[0].ACLPolicy.AllowedMethods.AdminService[0])
	require.Equal(t, "namespace1", converted.ClusterConnections[0].ACLPolicy.AllowedNamespaces[0])
	require.Equal(t, int64(100), converted.ClusterConnections[0].FVITranslation.Remote)
}

func TestConversionWithTLS(t *testing.T) {
	samplePath := filepath.Join("..", "develop", "legacyconfig", "old-config-with-TLS.yaml")

	proxyConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	require.NoError(t, err)
	converted := ToClusterConnConfig(proxyConfig)
	require.Equal(t, 0.1, converted.LogConfigs["adminservice"].ThrottleMaxRPS)
	require.Equal(t, float64(11), converted.LogConfigs["testexample"].ThrottleMaxRPS)
	require.Equal(t, 0.12, converted.LogConfigs["adminstreams"].ThrottleMaxRPS)
	require.Equal(t, false, converted.LogConfigs["adminstreams"].Disabled)
	require.Equal(t, true, converted.LogConfigs["testdisabled"].Disabled)
	require.Equal(t, 1, len(converted.ClusterConnections))
	require.Nil(t, converted.Inbound)
	require.Nil(t, converted.Outbound)
	require.Equal(t, ConnTypeMuxClient, converted.ClusterConnections[0].Remote.ConnectionType)
	require.True(t, converted.ClusterConnections[0].Remote.TcpServer.TLSConfig.SkipCAVerification)
	require.Equal(t, ConnTypeTCP, converted.ClusterConnections[0].Local.ConnectionType)
	require.Equal(t, "AddOrUpdateRemoteCluster", converted.ClusterConnections[0].ACLPolicy.AllowedMethods.AdminService[0])
	require.Equal(t, 0, len(converted.ClusterConnections[0].ACLPolicy.AllowedNamespaces))
	require.Equal(t, IntMapping{0, 0}, converted.ClusterConnections[0].FVITranslation)
}

func TestConversionWithOverride(t *testing.T) {
	samplePath := filepath.Join("..", "develop", "legacyconfig", "old-config-with-override.yaml")

	proxyConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	require.NoError(t, err)
	converted := ToClusterConnConfig(proxyConfig)
	require.Equal(t, 1, len(converted.ClusterConnections))
	require.Nil(t, converted.Inbound)
	require.Nil(t, converted.Outbound)
	require.Equal(t, ConnTypeMuxClient, converted.ClusterConnections[0].Remote.ConnectionType)
	require.True(t, converted.ClusterConnections[0].Remote.TcpServer.TLSConfig.SkipCAVerification)
	require.Equal(t, ConnTypeTCP, converted.ClusterConnections[0].Local.ConnectionType)
	require.Equal(t, "AddOrUpdateRemoteCluster", converted.ClusterConnections[0].ACLPolicy.AllowedMethods.AdminService[0])
	require.Equal(t, 0, len(converted.ClusterConnections[0].ACLPolicy.AllowedNamespaces))
	require.Equal(t, "127.0.0.1:6233", converted.ClusterConnections[0].ReplicationEndpoint)
}

func TestDefaultChart(t *testing.T) {
	samplePath := filepath.Join("..", "charts", "s2s-proxy", "files", "default.yaml")
	proxyConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	require.NoError(t, err)
	require.Equal(t, 1, len(proxyConfig.ClusterConnections))
	cc := proxyConfig.ClusterConnections[0]
	require.Equal(t, ConnectionType("tcp"), cc.Local.ConnectionType)
	require.Equal(t, "0.0.0.0:9233", cc.Local.TcpServer.ConnectionString)
	require.Equal(t, "frontend-ingress.temporal.svc.cluster.local:7233", cc.Local.TcpClient.ConnectionString)
	require.Equal(t, ConnectionType("mux-client"), cc.Remote.ConnectionType)
	require.Equal(t, "remote_proxy_service:8233", cc.Remote.MuxAddressInfo.ConnectionString)
	require.Equal(t, "my-s2s-proxy.svc.cluster.local:9233", cc.ReplicationEndpoint)
	require.False(t, cc.Remote.MuxAddressInfo.TLSConfig.IsEnabled())
}

func TestExampleChart(t *testing.T) {
	samplePath := filepath.Join("..", "charts", "s2s-proxy", "example.yaml")
	data, err := os.ReadFile(samplePath)
	require.NoError(t, err)

	// Split the multi-document YAML into individual manifests
	manifests := releaseutil.SplitManifests(string(data))

	// Find the ConfigMap manifest and extract config.yaml
	var configYAML string
	for _, manifest := range manifests {
		var doc struct {
			Kind string            `yaml:"kind"`
			Data map[string]string `yaml:"data"`
		}
		if err := yaml.Unmarshal([]byte(manifest), &doc); err != nil {
			continue
		}
		if doc.Kind == "ConfigMap" {
			configYAML = doc.Data["config.yaml"]
			break
		}
	}
	require.NotEmpty(t, configYAML, "config.yaml not found in ConfigMap")

	// Parse the S2SProxyConfig
	var proxyConfig S2SProxyConfig
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(configYAML)))
	decoder.KnownFields(true)
	err = decoder.Decode(&proxyConfig)
	require.NoError(t, err)

	// Verify the parsed config
	require.Equal(t, 1, len(proxyConfig.ClusterConnections))
	cc := proxyConfig.ClusterConnections[0]
	require.Equal(t, "my-migration-cluster", cc.Name)
	require.Equal(t, ConnectionType("tcp"), cc.Local.ConnectionType)
	// This value is overridden
	require.Equal(t, "frontend-address:7233", cc.Local.TcpClient.ConnectionString)
	require.Equal(t, ConnectionType("mux-client"), cc.Remote.ConnectionType)
	require.Equal(t, "s2s-proxy-sample.example.tmprl.cloud:8233", cc.Remote.MuxAddressInfo.ConnectionString)
}
