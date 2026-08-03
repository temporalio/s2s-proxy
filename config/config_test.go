package config

import (
	"bytes"
	"maps"
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

// ToMap returns the custom search attributes keyed by namespace name, where each inner map is
// customer-provided search attribute name -> internal field name. Test-only: production code
// reads the config through Aliases.
func (c *CustomSAConfig) ToMap() map[string]map[string]string {
	out := make(map[string]map[string]string, len(c.NamespaceMappings))
	for _, ns := range c.NamespaceMappings {
		attrs := make(map[string]string, len(ns.CustomSearchAttributes))
		maps.Copy(attrs, ns.CustomSearchAttributes)
		out[ns.Name] = attrs
	}
	return out
}

// Get returns the internal field name for the given customer-provided search attribute name
// in the given namespace. Test-only: production code reads the config through Aliases.
func (c *CustomSAConfig) Get(namespace string, searchAttr string) (string, bool) {
	for _, ns := range c.NamespaceMappings {
		if ns.Name != namespace {
			continue
		}
		internalName, found := ns.CustomSearchAttributes[searchAttr]
		return internalName, found
	}
	return "", false
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

	customSAs := cc.CustomSearchAttributes
	require.True(t, customSAs.IsEnabled())
	require.Equal(t, map[string]map[string]string{
		"namespace1": {
			"CustomKeywordField": "Keyword02",
			"CustomStringField":  "Text02",
			"MyKeyword":          "Keyword01",
			"MyText":             "Text01",
		},
	}, customSAs.ToMap())
	require.Equal(t, NewTuple("Keyword02", true), NewTuple(customSAs.Get("namespace1", "CustomKeywordField")))
	require.Equal(t, NewTuple("Text01", true), NewTuple(customSAs.Get("namespace1", "MyText")))
	require.Equal(t, NewTuple("", false), NewTuple(customSAs.Get("namespace1", "UnknownField")))
	require.Equal(t, NewTuple("", false), NewTuple(customSAs.Get("unknownNamespace", "MyText")))
	require.Equal(t, map[string]string{
		"Keyword02": "CustomKeywordField",
		"Text02":    "CustomStringField",
		"Keyword01": "MyKeyword",
		"Text01":    "MyText",
	}, customSAs.Aliases("namespace1"))
}

func TestCustomSAConfigAliases(t *testing.T) {
	cfg := CustomSAConfig{
		NamespaceMappings: []CustomSANamespaceMapping{
			{
				Name: "namespace1",
				CustomSearchAttributes: map[string]string{
					"MyKeyword": "Keyword01",
					"MyText":    "Text01",
				},
			},
			{
				Name: "namespace2",
				CustomSearchAttributes: map[string]string{
					"OtherKeyword": "Keyword01",
				},
			},
			{
				Name:                   "emptyNamespace",
				CustomSearchAttributes: map[string]string{},
			},
			{
				Name: "nilNamespace",
			},
		},
	}

	cases := []struct {
		name      string
		namespace string
		expected  map[string]string
	}{
		{
			name:      "reverses the configured mapping",
			namespace: "namespace1",
			expected: map[string]string{
				"Keyword01": "MyKeyword",
				"Text01":    "MyText",
			},
		},
		{
			// Each namespace is independent: namespace2 reuses Keyword01 without
			// affecting namespace1.
			name:      "scoped per namespace",
			namespace: "namespace2",
			expected:  map[string]string{"Keyword01": "OtherKeyword"},
		},
		{
			name:      "unknown namespace",
			namespace: "unknownNamespace",
			expected:  nil,
		},
		{
			name:      "namespace with empty mapping",
			namespace: "emptyNamespace",
			expected:  nil,
		},
		{
			name:      "namespace with nil mapping",
			namespace: "nilNamespace",
			expected:  nil,
		},
		{
			name:      "empty namespace name",
			namespace: "",
			expected:  nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, cfg.Aliases(c.namespace))
		})
	}
}

func TestCustomSAConfigAliasesEmptyConfig(t *testing.T) {
	var cfg CustomSAConfig
	require.False(t, cfg.IsEnabled())
	require.Nil(t, cfg.Aliases("namespace1"))
}

// Two search attribute names mapped to the same internal field name cannot both be
// represented in the reversed map, so one of them is dropped. Which one survives is
// not deterministic, so this only pins down that the result stays well formed.
func TestCustomSAConfigAliasesDuplicateInternalName(t *testing.T) {
	cfg := CustomSAConfig{
		NamespaceMappings: []CustomSANamespaceMapping{
			{
				Name: "namespace1",
				CustomSearchAttributes: map[string]string{
					"MyKeyword":    "Keyword01",
					"OtherKeyword": "Keyword01",
				},
			},
		},
	}

	aliases := cfg.Aliases("namespace1")
	require.Len(t, aliases, 1)
	require.Contains(t, []string{"MyKeyword", "OtherKeyword"}, aliases["Keyword01"])
}

// The first mapping wins when a namespace is listed more than once.
func TestCustomSAConfigAliasesDuplicateNamespace(t *testing.T) {
	cfg := CustomSAConfig{
		NamespaceMappings: []CustomSANamespaceMapping{
			{
				Name:                   "namespace1",
				CustomSearchAttributes: map[string]string{"MyKeyword": "Keyword01"},
			},
			{
				Name:                   "namespace1",
				CustomSearchAttributes: map[string]string{"MyText": "Text01"},
			},
		},
	}

	require.Equal(t, map[string]string{"Keyword01": "MyKeyword"}, cfg.Aliases("namespace1"))
}

// Aliases must not alias the configured map: mutating the result cannot corrupt config.
func TestCustomSAConfigAliasesReturnsCopy(t *testing.T) {
	cfg := CustomSAConfig{
		NamespaceMappings: []CustomSANamespaceMapping{
			{
				Name:                   "namespace1",
				CustomSearchAttributes: map[string]string{"MyKeyword": "Keyword01"},
			},
		},
	}

	aliases := cfg.Aliases("namespace1")
	aliases["Keyword01"] = "mutated"
	delete(aliases, "Keyword01")

	require.Equal(t, map[string]string{"Keyword01": "MyKeyword"}, cfg.Aliases("namespace1"))
	require.Equal(t, map[string]string{"MyKeyword": "Keyword01"}, cfg.NamespaceMappings[0].CustomSearchAttributes)
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
