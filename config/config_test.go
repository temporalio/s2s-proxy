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

func TestProxyAdminConfig(t *testing.T) {
	load := func(t *testing.T, body string) S2SProxyConfig {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		cfg, err := LoadConfig[S2SProxyConfig](path)
		require.NoError(t, err)
		return cfg
	}
	loadErr := func(t *testing.T, body string) error {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
		_, err := LoadConfig[S2SProxyConfig](path)
		return err
	}

	const clusterConnections = `
clusterConnections:
  - name: only
`

	t.Run("absent means disabled", func(t *testing.T) {
		cfg := load(t, clusterConnections)
		assert.Empty(t, cfg.ProxyAdmin.ListenAddress)
		assert.Nil(t, cfg.ProxyAdmin.Peer)
		assert.NoError(t, cfg.Validate())
	})

	t.Run("listen address round-trips", func(t *testing.T) {
		cfg := load(t, clusterConnections+"proxyAdmin:\n  listenAddress: \"localhost:6061\"\n")
		assert.Equal(t, "localhost:6061", cfg.ProxyAdmin.ListenAddress)
		assert.NoError(t, cfg.Validate())
	})

	// KnownFields(true) makes an unrecognized key fatal.
	// The Go field has to exist before any YAML can set it.
	t.Run("unknown key under proxyAdmin is rejected", func(t *testing.T) {
		require.Error(t, loadErr(t, clusterConnections+"proxyAdmin:\n  nope: 1\n"))
	})

	t.Run("unknown key under discovery is rejected", func(t *testing.T) {
		require.Error(t, loadErr(t, clusterConnections+`
proxyAdmin:
  peer:
    listenAddress: "127.0.0.1:9234"
    discovery:
      provider: dns
      nmae: typo
`))
	})

	// Every layered configuration tool deep-merges and cannot delete keys.
	// Switching provider leaves the previous provider's block behind.
	// It has to be inert rather than fatal, or there is no way to change provider through a Helm override at all.
	t.Run("an unselected provider's block is inert", func(t *testing.T) {
		cfg := load(t, clusterConnections+`
proxyAdmin:
  peer:
    listenAddress: "127.0.0.1:9234"
    discovery:
      provider: static
      dns:
        name: leftover.svc.cluster.local
      static:
        addresses: ["a:9234", "b:9234"]
`)
		require.NoError(t, cfg.Validate())
		assert.Equal(t, DiscoveryStatic, cfg.ProxyAdmin.Peer.Discovery.Provider)
		assert.Equal(t, "leftover.svc.cluster.local", cfg.ProxyAdmin.Peer.Discovery.DNS.Name)
	})

	t.Run("peer port defaults the dns port", func(t *testing.T) {
		cfg := load(t, clusterConnections+`
proxyAdmin:
  peer:
    listenAddress: "127.0.0.1:9234"
    discovery:
      provider: dns
      dns:
        name: peers.svc.cluster.local
`)
		require.NoError(t, cfg.Validate())
		assert.Equal(t, 9234, cfg.ProxyAdmin.Peer.PeerPort())
	})

	t.Run("validation rejects unusable configurations", func(t *testing.T) {
		for name, tc := range map[string]struct{ body, wants string }{
			// The operator listener has no TLS and no authorization.
			// Its View is a no-op.
			// Off-host is never a shape it can safely take.
			"operator listener off loopback": {
				body:  "proxyAdmin:\n  listenAddress: \"0.0.0.0:6061\"\n",
				wants: "proxyAdmin: listenAddress: is \"0.0.0.0:6061\", which is not loopback",
			},
			"operator listener without a port": {
				body:  "proxyAdmin:\n  listenAddress: \"localhost\"\n",
				wants: "proxyAdmin: listenAddress: is not a valid host:port",
			},
			"peer without a listen address": {
				body:  "proxyAdmin:\n  peer: {}\n",
				wants: "proxyAdmin.peer: listenAddress: is required",
			},
			"unknown provider": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"127.0.0.1:9234\"\n    discovery:\n      provider: carrier-pigeon\n",
				wants: "proxyAdmin.peer.discovery: provider: is \"carrier-pigeon\", want one of",
			},
			"dns without a name": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"127.0.0.1:9234\"\n    discovery:\n      provider: dns\n",
				wants: "proxyAdmin.peer.discovery: dns.name: is required",
			},
			// Siblings are dialed at the peer listen address port.
			// An address that binds an arbitrary port leaves discovery with nothing to dial.
			"dns without a port to dial": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"127.0.0.1:0\"\n    allowInsecure: true\n    discovery:\n      provider: dns\n      dns:\n        name: peers.svc\n",
				wants: "proxyAdmin.peer: discovery.dns.port: is required",
			},
			"static without addresses": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"127.0.0.1:9234\"\n    discovery:\n      provider: static\n",
				wants: "proxyAdmin.peer.discovery: static.addresses: is required",
			},
			// A plaintext listener on the pod network publishes the deployment's topology to anything that can reach it.
			// It has to be stated rather than fallen into.
			"non-loopback without tls": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"0.0.0.0:9234\"\n",
				wants: "set proxyAdmin.peer.allowInsecure to accept it",
			},
			// TLSConfig.IsEnabled is true with only caServerName set.
			// That would hand the listener a TLS config with no certificate and fail every handshake at runtime.
			"tls without a certificate": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"0.0.0.0:9234\"\n    tls:\n      caServerName: peers\n",
				wants: "proxyAdmin.peer: tls.certificatePath: is required",
			},
			// peerServerTLSConfig verifies its callers.
			// It needs the CA to verify them against.
			// Without this the listener fails to build and is skipped.
			// The only symptom is one log line plus every sibling reporting this pod unreachable.
			"tls without a CA to verify peers": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"0.0.0.0:9234\"\n    tls:\n      certificatePath: /c\n      keyPath: /k\n      caServerName: peers\n",
				wants: "proxyAdmin.peer: tls.remoteCAPath: is required",
			},
			// Siblings are dialed by IP.
			// Without this the fan-out dial has no name to verify the sibling certificate against.
			// It fails inside GetClientTLSConfig.
			"tls without a caServerName": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"0.0.0.0:9234\"\n    tls:\n      certificatePath: /c\n      keyPath: /k\n      remoteCAPath: /ca\n",
				wants: "proxyAdmin.peer: tls.caServerName: is required",
			},
			// GetClientTLSConfig assigns this to InsecureSkipVerify.
			// Every sibling this pod dials goes unverified while the config still reads as TLS-secured.
			"tls with verification skipped": {
				body:  "proxyAdmin:\n  peer:\n    listenAddress: \"0.0.0.0:9234\"\n    tls:\n      certificatePath: /c\n      keyPath: /k\n      remoteCAPath: /ca\n      caServerName: peers\n      skipCAVerification: true\n",
				wants: "proxyAdmin.peer: tls.skipCAVerification: disables verification",
			},
		} {
			t.Run(name, func(t *testing.T) {
				cfg := load(t, clusterConnections+tc.body)
				err := cfg.Validate()
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wants)
			})
		}
	})

	t.Run("non-loopback with allowInsecure is accepted", func(t *testing.T) {
		cfg := load(t, clusterConnections+"proxyAdmin:\n  peer:\n    listenAddress: \"0.0.0.0:9234\"\n    allowInsecure: true\n")
		require.NoError(t, cfg.Validate())
	})

	t.Run("a fully specified peer is accepted", func(t *testing.T) {
		cfg := load(t, clusterConnections+`
proxyAdmin:
  listenAddress: "127.0.0.1:6061"
  peer:
    listenAddress: "0.0.0.0:9234"
    tls:
      certificatePath: /c
      keyPath: /k
      remoteCAPath: /ca
      caServerName: peers
    discovery:
      provider: dns
      dns:
        name: peers.svc.cluster.local
`)
		require.NoError(t, cfg.Validate())
	})
}
