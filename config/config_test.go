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

// ToMap returns the custom search attribute aliases keyed by namespace name, where each inner
// map is internal field name -> customer-provided search attribute name. Test-only: production
// code reads the config through Aliases.
func (c *CustomSAAliasConfig) ToMap() map[string]map[string]string {
	out := make(map[string]map[string]string, len(c.NamespaceMappings))
	for _, ns := range c.NamespaceMappings {
		aliases := make(map[string]string, len(ns.CustomSearchAttributeAliases))
		maps.Copy(aliases, ns.CustomSearchAttributeAliases)
		out[ns.Name] = aliases
	}
	return out
}

// Get returns the customer-provided search attribute name aliased to the given internal field
// name in the given namespace. Test-only: production code reads the config through Aliases.
func (c *CustomSAAliasConfig) Get(namespace string, internalName string) (string, bool) {
	for _, ns := range c.NamespaceMappings {
		if ns.Name != namespace {
			continue
		}
		alias, found := ns.CustomSearchAttributeAliases[internalName]
		return alias, found
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

	customSAAliases := cc.CustomSearchAttributeAliases
	require.True(t, customSAAliases.IsEnabled())
	require.Equal(t, map[string]map[string]string{
		"namespace1": {
			"Keyword01": "MyKeyword",
			"Keyword02": "CustomKeywordField",
			"Text01":    "MyText",
			"Text02":    "CustomStringField",
		},
	}, customSAAliases.ToMap())
	require.Equal(t, NewTuple("CustomKeywordField", true), NewTuple(customSAAliases.Get("namespace1", "Keyword02")))
	require.Equal(t, NewTuple("MyText", true), NewTuple(customSAAliases.Get("namespace1", "Text01")))
	require.Equal(t, NewTuple("", false), NewTuple(customSAAliases.Get("namespace1", "UnknownField")))
	require.Equal(t, NewTuple("", false), NewTuple(customSAAliases.Get("unknownNamespace", "Text01")))
	require.Equal(t, map[string]string{
		"Keyword01": "MyKeyword",
		"Keyword02": "CustomKeywordField",
		"Text01":    "MyText",
		"Text02":    "CustomStringField",
	}, customSAAliases.Aliases("namespace1"))
}

func TestCustomSAAliasConfigAliases(t *testing.T) {
	cfg := CustomSAAliasConfig{
		NamespaceMappings: []CustomSAAliasNamespaceMapping{
			{
				Name: "namespace1",
				CustomSearchAttributeAliases: map[string]string{
					"Keyword01": "MyKeyword",
					"Text01":    "MyText",
				},
			},
			{
				Name: "namespace2",
				CustomSearchAttributeAliases: map[string]string{
					"Keyword01": "OtherKeyword",
				},
			},
			{
				Name:                         "emptyNamespace",
				CustomSearchAttributeAliases: map[string]string{},
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
			name:      "returns the configured aliases",
			namespace: "namespace1",
			expected: map[string]string{
				"Keyword01": "MyKeyword",
				"Text01":    "MyText",
			},
		},
		{
			// Each namespace is independent: namespace2 aliases Keyword01 to a different
			// name without affecting namespace1.
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

func TestCustomSAAliasConfigAliasesEmptyConfig(t *testing.T) {
	var cfg CustomSAAliasConfig
	require.False(t, cfg.IsEnabled())
	require.Nil(t, cfg.Aliases("namespace1"))
}

// Distinct internal fields may share an alias name. Both entries are preserved; the server is
// responsible for rejecting the namespace config if that is invalid.
func TestCustomSAAliasConfigAliasesDuplicateAliasName(t *testing.T) {
	cfg := CustomSAAliasConfig{
		NamespaceMappings: []CustomSAAliasNamespaceMapping{
			{
				Name: "namespace1",
				CustomSearchAttributeAliases: map[string]string{
					"Keyword01": "MyKeyword",
					"Keyword02": "MyKeyword",
				},
			},
		},
	}

	require.Equal(t, map[string]string{
		"Keyword01": "MyKeyword",
		"Keyword02": "MyKeyword",
	}, cfg.Aliases("namespace1"))
}

// The first mapping wins when a namespace is listed more than once.
func TestCustomSAAliasConfigAliasesDuplicateNamespace(t *testing.T) {
	cfg := CustomSAAliasConfig{
		NamespaceMappings: []CustomSAAliasNamespaceMapping{
			{
				Name:                         "namespace1",
				CustomSearchAttributeAliases: map[string]string{"Keyword01": "MyKeyword"},
			},
			{
				Name:                         "namespace1",
				CustomSearchAttributeAliases: map[string]string{"Text01": "MyText"},
			},
		},
	}

	require.Equal(t, map[string]string{"Keyword01": "MyKeyword"}, cfg.Aliases("namespace1"))
}

// Aliases must not alias the configured map: mutating the result cannot corrupt config.
func TestCustomSAAliasConfigAliasesReturnsCopy(t *testing.T) {
	cfg := CustomSAAliasConfig{
		NamespaceMappings: []CustomSAAliasNamespaceMapping{
			{
				Name:                         "namespace1",
				CustomSearchAttributeAliases: map[string]string{"Keyword01": "MyKeyword"},
			},
		},
	}

	aliases := cfg.Aliases("namespace1")
	aliases["Keyword01"] = "mutated"
	delete(aliases, "Keyword01")

	require.Equal(t, map[string]string{"Keyword01": "MyKeyword"}, cfg.Aliases("namespace1"))
	require.Equal(t, map[string]string{"Keyword01": "MyKeyword"}, cfg.NamespaceMappings[0].CustomSearchAttributeAliases)
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

func TestSATranslationConfigValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  SATranslationConfig
		// wantValidateErr lists substrings the Validate error must contain. Empty means the
		// config is valid.
		wantValidateErr []string
		// wantValidateErrExcludes lists substrings the Validate error must not contain.
		wantValidateErrExcludes []string
		// wantTranslationErr lists substrings the AsLocalToRemoteSATranslation error must
		// contain. Leave nil when the translation fails for the same reason as Validate.
		wantTranslationErr []string
		// verify runs against the built translation when it is expected to succeed.
		verify func(t *testing.T, saTranslation SearchAttributeTranslation)
	}{
		{
			name: "no namespace mappings",
			cfg:  SATranslationConfig{},
			verify: func(t *testing.T, saTranslation SearchAttributeTranslation) {
				require.Equal(t, 0, saTranslation.LenNamespaces())
			},
		},
		{
			// Two namespaces sharing an id used to silently overwrite each other, leaving one
			// namespace translated with the other namespace's mappings.
			name: "duplicate namespaceId",
			cfg: SATranslationConfig{
				NamespaceMappings: []SANamespaceMapping{
					{
						Name:        "namespace1",
						NamespaceId: "namespace-id-1",
						Mappings:    []SAMapping{{LocalName: "localOne", RemoteName: "remoteOne"}},
					},
					{
						Name:        "namespace2",
						NamespaceId: "namespace-id-1",
						Mappings:    []SAMapping{{LocalName: "localTwo", RemoteName: "remoteTwo"}},
					},
				},
			},
			wantValidateErr: []string{
				`namespaceMappings[name="namespace1"]`,
				`namespaceMappings[name="namespace2"]`,
				`duplicate namespaceId "namespace-id-1"`,
			},
		},
		{
			name: "empty namespaceId alongside another namespace",
			cfg: SATranslationConfig{
				NamespaceMappings: []SANamespaceMapping{
					{
						Name:     "legacyNamespace",
						Mappings: []SAMapping{{LocalName: "localOne", RemoteName: "remoteOne"}},
					},
					{
						Name:        "namespace2",
						NamespaceId: "namespace-id-2",
						Mappings:    []SAMapping{{LocalName: "localTwo", RemoteName: "remoteTwo"}},
					},
				},
			},
			wantValidateErr: []string{
				`namespaceMappings[name="legacyNamespace"]`,
				"has no namespaceId",
			},
		},
		{
			// The missing id is the actionable problem, so it is reported ahead of the duplicate
			// id the empty values also form.
			name: "every mapping missing its namespaceId",
			cfg: SATranslationConfig{
				NamespaceMappings: []SANamespaceMapping{
					{
						Name:     "namespace1",
						Mappings: []SAMapping{{LocalName: "localOne", RemoteName: "remoteOne"}},
					},
					{
						Name:     "namespace2",
						Mappings: []SAMapping{{LocalName: "localTwo", RemoteName: "remoteTwo"}},
					},
				},
			},
			wantValidateErr: []string{
				`namespaceMappings[name="namespace1"]`,
				"has no namespaceId",
			},
			wantValidateErrExcludes: []string{"duplicate namespaceId"},
		},
		{
			// namespaceId is required even for one namespace: there is no mapping that applies
			// to every namespace, so an omitted id has nothing to match against.
			name: "single mapping with empty namespaceId",
			cfg: SATranslationConfig{
				NamespaceMappings: []SANamespaceMapping{
					{
						Mappings: []SAMapping{{LocalName: "localOne", RemoteName: "remoteOne"}},
					},
				},
			},
			wantValidateErr: []string{"has no namespaceId", "namespaceId is required"},
		},
		{
			// The shape the migration tooling emits today: the namespace is named but the
			// namespaceId is blank. It must fail at startup naming the entry, rather than
			// translating some arbitrary namespace.
			name: "named mapping with empty namespaceId",
			cfg: SATranslationConfig{
				NamespaceMappings: []SANamespaceMapping{
					{
						Name: "migration-namespace",
						Mappings: []SAMapping{
							{LocalName: "CustomKeywordField", RemoteName: "Keyword01"},
							{LocalName: "CustomStringField", RemoteName: "Text01"},
						},
					},
				},
			},
			wantValidateErr: []string{
				`namespaceMappings[name="migration-namespace"]`,
				"has no namespaceId",
			},
		},
		{
			// The namespace is well formed, the mappings inside it are not: the bimap rejects
			// them and the error must say which namespace it came from.
			name: "duplicate localFieldName within one namespace",
			cfg: SATranslationConfig{
				NamespaceMappings: []SANamespaceMapping{
					{
						Name:        "namespace1",
						NamespaceId: "namespace-id-1",
						Mappings: []SAMapping{
							{LocalName: "localOne", RemoteName: "remoteOne"},
							{LocalName: "localOne", RemoteName: "remoteTwo"},
						},
					},
				},
			},
			wantTranslationErr: []string{
				`namespaceMappings[name="namespace1" namespaceId="namespace-id-1"]`,
			},
		},
		{
			name: "duplicate name across namespaces",
			cfg: SATranslationConfig{
				NamespaceMappings: []SANamespaceMapping{
					{
						Name:        "namespace1",
						NamespaceId: "namespace-id-1",
						Mappings:    []SAMapping{{LocalName: "localOne", RemoteName: "remoteOne"}},
					},
					{
						Name:        "namespace1",
						NamespaceId: "namespace-id-2",
						Mappings:    []SAMapping{{LocalName: "localTwo", RemoteName: "remoteTwo"}},
					},
				},
			},
			wantValidateErr: []string{`duplicate name "namespace1"`},
		},
		{
			name: "distinct namespaceIds translate independently",
			cfg: SATranslationConfig{
				NamespaceMappings: []SANamespaceMapping{
					{
						Name:        "namespace1",
						NamespaceId: "namespace-id-1",
						Mappings:    []SAMapping{{LocalName: "localOne", RemoteName: "remoteOne"}},
					},
					{
						Name:        "namespace2",
						NamespaceId: "namespace-id-2",
						Mappings:    []SAMapping{{LocalName: "localTwo", RemoteName: "remoteTwo"}},
					},
				},
			},
			verify: func(t *testing.T, saTranslation SearchAttributeTranslation) {
				require.Equal(t, 2, saTranslation.LenNamespaces())
				require.Equal(t, "remoteOne", saTranslation.Get("namespace-id-1", "localOne"))
				require.Equal(t, "remoteTwo", saTranslation.Get("namespace-id-2", "localTwo"))
				// Each namespace only knows its own attributes.
				require.Equal(t, "", saTranslation.Get("namespace-id-1", "localTwo"))
				require.Equal(t, "", saTranslation.Get("namespace-id-2", "localOne"))
				require.Equal(t, NewTuple("", false), NewTuple(saTranslation.GetExists("", "localOne")))
				require.Equal(t, "localOne", saTranslation.Inverse().Get("namespace-id-1", "remoteOne"))
				require.Equal(t, "localTwo", saTranslation.Inverse().Get("namespace-id-2", "remoteTwo"))
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := c.cfg
			err := cfg.Validate()
			if len(c.wantValidateErr) == 0 {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				for _, want := range c.wantValidateErr {
					require.Contains(t, err.Error(), want)
				}
				for _, unwanted := range c.wantValidateErrExcludes {
					require.NotContains(t, err.Error(), unwanted)
				}
			}

			// Validate runs inside AsLocalToRemoteSATranslation, so an invalid config fails
			// there for the same reason unless the case says otherwise.
			wantTranslationErr := c.wantTranslationErr
			if wantTranslationErr == nil {
				wantTranslationErr = c.wantValidateErr
			}
			saTranslation, err := cfg.AsLocalToRemoteSATranslation()
			if len(wantTranslationErr) == 0 {
				require.NoError(t, err)
				c.verify(t, saTranslation)
				return
			}
			require.Error(t, err)
			for _, want := range wantTranslationErr {
				require.Contains(t, err.Error(), want)
			}
			require.Equal(t, 0, saTranslation.LenNamespaces())
		})
	}
}
