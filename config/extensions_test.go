package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/validation"

	"github.com/temporalio/s2s-proxy/encryption"
)

func TestExtensionServerValidate(t *testing.T) {
	cases := []struct {
		name   string
		server ExtensionServer
		want   validation.Errors
	}{
		{
			name:   "valid server",
			server: validExtensionServer(),
		},
		{
			name:   "name is required",
			server: ExtensionServer{Address: "127.0.0.1:9443"},
			want: validation.Errors{
				{Field: "name", Message: "is required"},
			},
		},
		{
			name:   "address is required",
			server: ExtensionServer{Name: "hsm"},
			want: validation.Errors{
				{Field: "address", Message: "is not a valid host:port"},
			},
		},
		{
			name:   "address must carry a port",
			server: ExtensionServer{Name: "hsm", Address: "127.0.0.1"},
			want: validation.Errors{
				{Field: "address", Message: "is not a valid host:port"},
			},
		},
		{
			name:   "address must not be a URL",
			server: ExtensionServer{Name: "hsm", Address: "https://hsm.internal:9443"},
			want: validation.Errors{
				{Field: "address", Message: "is not a valid host:port"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.server.Validate()
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireErrors(t, err, tc.want)
		})
	}
}

func TestExtensionServerListValidate(t *testing.T) {
	cases := []struct {
		name    string
		servers ExtensionServerList
		want    validation.Errors
	}{
		{
			name: "nil list is valid",
		},
		{
			name:    "empty list is valid",
			servers: ExtensionServerList{},
		},
		{
			name: "several distinct servers are valid",
			servers: ExtensionServerList{
				{Name: "hsm", Address: "127.0.0.1:9443"},
				{Name: "vault", Address: "127.0.0.1:9444"},
			},
		},
		{
			name: "duplicate names belong to the collection",
			servers: ExtensionServerList{
				{Name: "hsm", Address: "127.0.0.1:9443"},
				{Name: "hsm", Address: "127.0.0.1:9444"},
			},
			want: validation.Errors{
				{Field: "[name]", Message: "contains duplicate value: hsm"},
			},
		},
		{
			name: "duplicate addresses belong to the collection",
			servers: ExtensionServerList{
				{Name: "hsm", Address: "127.0.0.1:9443"},
				{Name: "vault", Address: "127.0.0.1:9443"},
			},
			want: validation.Errors{
				{Field: "[address]", Message: "contains duplicate value: 127.0.0.1:9443"},
			},
		},
		{
			name: "per-entry failures are stamped with their index",
			servers: ExtensionServerList{
				{Name: "hsm", Address: "127.0.0.1:9443"},
				{Address: "127.0.0.1:9444"},
			},
			want: validation.Errors{
				{Subject: "[1]", Field: "name", Message: "is required"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.servers.Validate()
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireErrors(t, err, tc.want)
		})
	}
}

func validExtensionServer() ExtensionServer {
	return ExtensionServer{Name: "hsm", Address: "127.0.0.1:9443"}
}

func TestExtensionServersFromYAML(t *testing.T) {
	path := writeYAML(t, `
extensionServers:
  - name: hsm
    address: 127.0.0.1:9443
    tls:
      certificatePath: /etc/certs/proxy.pem
      keyPath: /etc/certs/proxy.key
      remoteCAPath: /etc/certs/ca.pem
      caServerName: hsm.internal
  - name: vault
    address: vault.internal:9444
`)

	cfg, err := LoadConfig[S2SProxyConfig](path)
	require.NoError(t, err)

	require.Equal(t, ExtensionServerList{
		{
			Name:    "hsm",
			Address: "127.0.0.1:9443",
			TLSConfig: encryption.TLSConfig{
				CertificatePath: "/etc/certs/proxy.pem",
				KeyPath:         "/etc/certs/proxy.key",
				RemoteCAPath:    "/etc/certs/ca.pem",
				CAServerName:    "hsm.internal",
			},
		},
		{Name: "vault", Address: "vault.internal:9444"},
	}, cfg.ExtensionServers)

	require.NoError(t, cfg.Validate())
}

func TestS2SProxyConfigValidateExtensionServers(t *testing.T) {
	cases := []struct {
		name    string
		servers ExtensionServerList
		want    validation.Errors
	}{
		{
			name: "absent list is valid",
		},
		{
			name:    "valid list",
			servers: ExtensionServerList{validExtensionServer()},
		},
		{
			name: "collection failures compose onto the field",
			servers: ExtensionServerList{
				{Name: "hsm", Address: "127.0.0.1:9443"},
				{Name: "hsm", Address: "127.0.0.1:9444"},
			},
			want: validation.Errors{
				{Field: "extensionServers[name]", Message: "contains duplicate value: hsm"},
			},
		},
		{
			name: "per-entry failures compose onto the subject",
			servers: ExtensionServerList{
				{Name: "hsm", Address: "127.0.0.1:9443"},
				{Name: "vault", Address: "nope"},
			},
			want: validation.Errors{
				{Subject: "extensionServers[1]", Field: "address", Message: "is not a valid host:port"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := S2SProxyConfig{ExtensionServers: tc.servers}

			err := cfg.Validate()
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireErrors(t, err, tc.want)
		})
	}
}

func TestKeyPolicyValidateExtensionURIs(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want validation.Errors
	}{
		{
			name: "server and key",
			uri:  "extension://hsm/replication",
		},
		{
			name: "server alone is enough",
			uri:  "extension://hsm",
		},
		{
			name: "scheme is case-insensitive",
			uri:  "EXTENSION://hsm/replication",
		},
		{
			name: "no server at all",
			uri:  "extension://",
			want: validation.Errors{
				{Field: "uri", Message: "extension key URI must name an extension server: extension://"},
			},
		},
		{
			name: "empty server with a key",
			uri:  "extension:///replication",
			want: validation.Errors{
				{Field: "uri", Message: "extension key URI must name an extension server: extension:///replication"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy := KeyPolicy{URI: tc.uri, Duration: time.Hour}

			err := policy.Validate()
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireErrors(t, err, tc.want)
		})
	}
}

func TestEncryptionConfigReferentialRules(t *testing.T) {
	known := map[string]struct{}{"hsm": {}}

	cases := []struct {
		name string
		cfg  EncryptionConfig
		want validation.Errors
	}{
		{
			name: "no default and no overrides",
			cfg:  EncryptionConfig{},
		},
		{
			name: "default names a configured server",
			cfg:  EncryptionConfig{Default: &KeyPolicy{URI: "extension://hsm/replication"}},
		},
		{
			name: "non-extension schemes are not references",
			cfg:  EncryptionConfig{Default: &KeyPolicy{URI: "awskms://alias/primary"}},
		},
		{
			name: "an empty host is left to the scheme check",
			cfg:  EncryptionConfig{Default: &KeyPolicy{URI: "extension://"}},
		},
		{
			name: "default names an unknown server",
			cfg:  EncryptionConfig{Default: &KeyPolicy{URI: "extension://vault/replication"}},
			want: validation.Errors{
				{Subject: "encryption.default", Field: "uri", Message: "unknown extension server: vault"},
			},
		},
		{
			name: "server names are matched case-sensitively",
			cfg:  EncryptionConfig{Default: &KeyPolicy{URI: "extension://HSM/replication"}},
			want: validation.Errors{
				{Subject: "encryption.default", Field: "uri", Message: "unknown extension server: HSM"},
			},
		},
		{
			name: "a decryptURI names an unknown server",
			cfg: EncryptionConfig{Default: &KeyPolicy{
				URI:         "extension://hsm/replication",
				DecryptURIs: []string{"extension://hsm/retired", "extension://vault/older"},
			}},
			want: validation.Errors{
				{Subject: "encryption.default", Field: "decryptURIs[1]", Message: "unknown extension server: vault"},
			},
		},
		{
			name: "an override names an unknown server",
			cfg: EncryptionConfig{
				Default:   &KeyPolicy{URI: "extension://hsm/replication"},
				Overrides: map[string]KeyPolicy{"tenant-a": {URI: "extension://vault/tenant-a"}},
			},
			want: validation.Errors{
				{Subject: "encryption.overrides[tenant-a]", Field: "uri", Message: "unknown extension server: vault"},
			},
		},
		{
			name: "overrides are reported in sorted order",
			cfg: EncryptionConfig{
				Overrides: map[string]KeyPolicy{
					"tenant-b": {URI: "extension://b/key"},
					"tenant-a": {URI: "extension://a/key"},
				},
			},
			want: validation.Errors{
				{Subject: "encryption.overrides[tenant-a]", Field: "uri", Message: "unknown extension server: a"},
				{Subject: "encryption.overrides[tenant-b]", Field: "uri", Message: "unknown extension server: b"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got validation.Errors
			for _, rule := range tc.cfg.referentialRules("encryption", known) {
				got = append(got, rule()...)
			}

			require.Equal(t, tc.want, got)
		})
	}
}

func TestS2SProxyConfigValidateExtensionReferences(t *testing.T) {
	policy := func(uri string) *KeyPolicy {
		return &KeyPolicy{URI: uri, Duration: 24 * time.Hour, RenewBefore: time.Hour}
	}
	conn := func(name, uri string) ClusterConnConfig {
		return ClusterConnConfig{
			Name:             name,
			EncryptionConfig: EncryptionConfig{Enabled: true, Default: policy(uri)},
		}
	}

	cases := []struct {
		name string
		cfg  S2SProxyConfig
		want validation.Errors
	}{
		{
			name: "a key URI naming a configured server",
			cfg: S2SProxyConfig{
				ExtensionServers:   ExtensionServerList{validExtensionServer()},
				ClusterConnections: []ClusterConnConfig{conn("a", "extension://hsm/replication")},
			},
		},
		{
			name: "one server backs several cluster connections",
			cfg: S2SProxyConfig{
				ExtensionServers: ExtensionServerList{validExtensionServer()},
				ClusterConnections: []ClusterConnConfig{
					conn("a", "extension://hsm/replication"),
					conn("b", "extension://hsm/replication"),
				},
			},
		},
		{
			name: "cloud KMS URIs need no extension server",
			cfg: S2SProxyConfig{
				ClusterConnections: []ClusterConnConfig{conn("a", "awskms://alias/primary")},
			},
		},
		{
			name: "a key URI naming no configured server",
			cfg: S2SProxyConfig{
				ClusterConnections: []ClusterConnConfig{conn("a", "extension://hsm/replication")},
			},
			want: validation.Errors{
				{
					Subject: "clusterConnections[0].encryption.default",
					Field:   "uri",
					Message: "unknown extension server: hsm",
				},
			},
		},
		{
			name: "the failing connection is identified by index",
			cfg: S2SProxyConfig{
				ExtensionServers: ExtensionServerList{validExtensionServer()},
				ClusterConnections: []ClusterConnConfig{
					conn("a", "extension://hsm/replication"),
					conn("b", "extension://vault/replication"),
				},
			},
			want: validation.Errors{
				{
					Subject: "clusterConnections[1].encryption.default",
					Field:   "uri",
					Message: "unknown extension server: vault",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if tc.want == nil {
				require.NoError(t, err)
				return
			}

			requireErrors(t, err, tc.want)
		})
	}
}

func TestS2SProxyConfigValidateExtensionReferencesFromYAML(t *testing.T) {
	path := writeYAML(t, `
extensionServers:
  - name: hsm
    address: 127.0.0.1:9443
clusterConnections:
  - name: a
    encryption:
      enabled: true
      default:
        uri: extension://vault/replication
        duration: 24h
        renewBefore: 1h
`)

	cfg, err := LoadConfig[S2SProxyConfig](path)
	require.NoError(t, err)

	requireErrors(t, cfg.Validate(), validation.Errors{
		{
			Subject: "clusterConnections[0].encryption.default",
			Field:   "uri",
			Message: "unknown extension server: vault",
		},
	})
}
