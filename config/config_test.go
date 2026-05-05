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

func TestLoadS2SConfigMux(t *testing.T) {
	cases := []struct {
		file       string
		remoteType ConnectionType
	}{
		{"cluster-a-mux-client-proxy.yaml", ConnTypeMuxClient},
		{"cluster-b-mux-server-proxy.yaml", ConnTypeMuxServer},
	}

	for _, c := range cases {
		samplePath := filepath.Join("..", "develop", "config", c.file)
		s2sConfig, err := LoadConfig[S2SProxyConfig](samplePath)
		require.NoError(t, err)
		require.Equal(t, 1, len(s2sConfig.ClusterConnections))
		assert.Equal(t, ConnTypeTCP, s2sConfig.ClusterConnections[0].Local.ConnectionType)
		assert.Equal(t, c.remoteType, s2sConfig.ClusterConnections[0].Remote.ConnectionType)
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

	cc := proxyConfig.ClusterConnections[0]
	require.Equal(t, "127.0.0.1:9002", cc.ReplicationEndpoint)
	require.Equal(t, IntMapping{Local: 100, Remote: 1000000}, cc.FVITranslation)
	require.NotNil(t, cc.ACLPolicy)
	require.Contains(t, cc.ACLPolicy.AllowedMethods.AdminService, "AddOrUpdateRemoteCluster")
	require.Contains(t, cc.ACLPolicy.AllowedMethods.AdminService, "StreamWorkflowReplicationMessages")
	require.Equal(t, []string{"namespace1", "namespace2"}, cc.ACLPolicy.AllowedNamespaces)
	require.True(t, cc.Remote.MuxAddressInfo.TLSConfig.SkipCAVerification)
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
