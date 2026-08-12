package config

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/validation"

	"github.com/temporalio/s2s-proxy/encryption"
)

func TestS2SProxyConfigValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  S2SProxyConfig
		want validation.Errors
	}{
		{
			name: "no cluster connections",
		},
		{
			name: "connection with no encryption block",
			cfg: S2SProxyConfig{
				ClusterConnections: []ClusterConnConfig{{Name: "cluster-a"}},
			},
		},
		{
			name: "valid encryption block",
			cfg: S2SProxyConfig{
				ClusterConnections: []ClusterConnConfig{{
					Name: "cluster-a",
					EncryptionConfig: EncryptionConfig{
						Enabled: true,
						Default: &KeyPolicy{URI: "awskms://primary", Duration: time.Hour},
					},
				}},
			},
		},
		{
			name: "every error carries a path back to its field",
			cfg: S2SProxyConfig{
				ClusterConnections: []ClusterConnConfig{{
					Name: "cluster-a",
					EncryptionConfig: EncryptionConfig{
						Enabled:   true,
						CacheSize: -1,
						Default: &KeyPolicy{
							URI:         "vault://primary",
							DecryptURIs: []string{"vault://retired"},
							Duration:    time.Hour,
						},
						Overrides: map[string]KeyPolicy{
							"tenant-a": {URI: "vault://tenant", Duration: time.Hour},
						},
					},
				}},
			},
			want: validation.Errors{
				{
					Subject: "clusterConnections[0].encryption",
					Field:   "cacheSize",
					Message: "not greater than or equal to 0",
				},
				{
					Subject: "clusterConnections[0].encryption.default",
					Field:   "uri",
					Message: invalidURI("vault://primary"),
				},
				{
					Subject: "clusterConnections[0].encryption.default.decryptURIs[0]",
					Message: invalidURI("vault://retired"),
				},
				{
					Subject: "clusterConnections[0].encryption.overrides[tenant-a]",
					Field:   "uri",
					Message: invalidURI("vault://tenant"),
				},
			},
		},
		{
			name: "connections are reported by index",
			cfg: S2SProxyConfig{
				ClusterConnections: []ClusterConnConfig{
					{Name: "cluster-a"},
					{
						Name: "cluster-b",
						EncryptionConfig: EncryptionConfig{
							Default: &KeyPolicy{URI: "vault://b", Duration: time.Hour},
						},
					},
					{
						Name:             "cluster-c",
						EncryptionConfig: EncryptionConfig{Enabled: true},
					},
				},
			},
			want: validation.Errors{
				{
					Subject: "clusterConnections[1].encryption.default",
					Field:   "uri",
					Message: invalidURI("vault://b"),
				},
				{
					Subject: "clusterConnections[2].encryption",
					Field:   "default",
					Message: "is required",
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if c.want == nil {
				require.NoError(t, err)
				return
			}

			requireErrors(t, err, c.want)
		})
	}
}

// TestS2SProxyConfigValidateFromYAML runs the whole path an operator hits: a
// config file with a typo in a key URI, loaded and then validated.
func TestS2SProxyConfigValidateFromYAML(t *testing.T) {
	path := writeYAML(t, `
clusterConnections:
  - name: cluster-a
    encryption:
      enabled: true
      default:
        uri: awskms://alias/primary
        duration: 24h
      overrides:
        tenant-a:
          uri: vault://alias/typo
          duration: 24h
`)

	cfg, err := LoadConfig[S2SProxyConfig](path)
	require.NoError(t, err)

	requireErrors(t, cfg.Validate(), validation.Errors{
		{
			Subject: "clusterConnections[0].encryption.overrides[tenant-a]",
			Field:   "uri",
			Message: invalidURI("vault://alias/typo"),
		},
	})
}

// notLoopback is the message isLoopback produces.
// The two call sites differ only by the remedy they name.
func notLoopback(listenAddress, remedy string) string {
	return fmt.Sprintf("is %q, which is not loopback: this publishes an unauthenticated view of "+
		"the deployment topology to anything that can reach it. %s", listenAddress, remedy)
}

const (
	operatorRemedy = "Bind it to loopback, or serve siblings through proxyAdmin.peer, which authenticates its callers."
	peerRemedy     = "Configure proxyAdmin.peer.tls, or set proxyAdmin.peer.allowInsecure to accept it."
)

func proxyAdmin(c ProxyAdminConfig) S2SProxyConfig {
	return S2SProxyConfig{ProxyAdmin: c}
}

func peerAt(listenAddress string) *ProxyAdminPeerConfig {
	return &ProxyAdminPeerConfig{ListenAddress: listenAddress}
}

func TestProxyAdminValidate(t *testing.T) {
	cases := []struct {
		name string
		cfg  S2SProxyConfig
		want validation.Errors
	}{
		{
			name: "absent",
		},
		{
			name: "operator listener on loopback",
			cfg:  proxyAdmin(ProxyAdminConfig{ListenAddress: "localhost:6061"}),
		},
		{
			// The operator listener has no TLS and no authorization.
			// Its View is a no-op.
			name: "operator listener off loopback",
			cfg:  proxyAdmin(ProxyAdminConfig{ListenAddress: "0.0.0.0:6061"}),
			want: validation.Errors{{
				Subject: "proxyAdmin",
				Field:   "listenAddress",
				Message: notLoopback("0.0.0.0:6061", operatorRemedy),
			}},
		},
		{
			// One message, not two.
			// "localhost" is loopback.
			// The missing port is the problem.
			name: "operator listener without a port",
			cfg:  proxyAdmin(ProxyAdminConfig{ListenAddress: "localhost"}),
			want: validation.Errors{{
				Subject: "proxyAdmin",
				Field:   "listenAddress",
				Message: "is not a valid host:port",
			}},
		},
		{
			name: "peer without a listen address",
			cfg:  proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{}}),
			want: validation.Errors{{
				Subject: "proxyAdmin.peer",
				Field:   "listenAddress",
				Message: "is required",
			}},
		},
		{
			name: "peer listen address is not host:port",
			cfg:  proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{ListenAddress: "peers.svc", AllowInsecure: true}}),
			want: validation.Errors{{
				Subject: "proxyAdmin.peer",
				Field:   "listenAddress",
				Message: "is not a valid host:port",
			}},
		},
		{
			// A plaintext listener on the pod network publishes the deployment's topology.
			// It has to be stated rather than fallen into.
			name: "peer off loopback with no tls",
			cfg:  proxyAdmin(ProxyAdminConfig{Peer: peerAt("0.0.0.0:9234")}),
			want: validation.Errors{{
				Subject: "proxyAdmin.peer",
				Field:   "listenAddress",
				Message: notLoopback("0.0.0.0:9234", peerRemedy),
			}},
		},
		{
			name: "peer off loopback with allowInsecure",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "0.0.0.0:9234", AllowInsecure: true,
			}}),
		},
		{
			// TLSConfig.IsEnabled is true with only caServerName set.
			// That would hand the listener a TLS config with no certificate.
			name: "tls with only a caServerName",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "0.0.0.0:9234",
				TLS:           &encryption.TLSConfig{CAServerName: "peers"},
			}}),
			want: validation.Errors{
				{Subject: "proxyAdmin.peer", Field: "tls.certificatePath", Message: "is required"},
				{Subject: "proxyAdmin.peer", Field: "tls.keyPath", Message: "is required"},
				{Subject: "proxyAdmin.peer", Field: "tls.remoteCAPath", Message: "is required"},
			},
		},
		{
			// Every failure is reported at once.
			// One load names every field an operator has to fix.
			//
			// The peer listener verifies its callers.
			// It needs the CA to verify them against.
			// Siblings are dialed by IP.
			// caServerName has to name a SAN every pod carries.
			name: "tls without a CA or a server name",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "0.0.0.0:9234",
				TLS:           &encryption.TLSConfig{CertificatePath: "/c", KeyPath: "/k"},
			}}),
			want: validation.Errors{
				{Subject: "proxyAdmin.peer", Field: "tls.remoteCAPath", Message: "is required"},
				{Subject: "proxyAdmin.peer", Field: "tls.caServerName", Message: "is required"},
			},
		},
		{
			// GetClientTLSConfig assigns this to InsecureSkipVerify.
			name: "tls with verification skipped",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "0.0.0.0:9234",
				TLS: &encryption.TLSConfig{
					CertificatePath: "/c", KeyPath: "/k", RemoteCAPath: "/ca",
					CAServerName: "peers", SkipCAVerification: true,
				},
			}}),
			want: validation.Errors{{
				Subject: "proxyAdmin.peer",
				Field:   "tls.skipCAVerification",
				Message: "disables verification of every sibling this pod dials, which defeats the peer TLS it is set alongside",
			}},
		},
		{
			name: "unknown discovery provider",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "127.0.0.1:9234",
				Discovery:     DiscoveryConfig{Provider: "carrier-pigeon"},
			}}),
			want: validation.Errors{{
				Subject: "proxyAdmin.peer.discovery",
				Field:   "provider",
				Message: `is "carrier-pigeon", want one of [none dns static]`,
			}},
		},
		{
			name: "dns provider without a name",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "127.0.0.1:9234",
				Discovery:     DiscoveryConfig{Provider: DiscoveryDNS},
			}}),
			want: validation.Errors{{
				Subject: "proxyAdmin.peer.discovery",
				Field:   "dns.name",
				Message: "is required",
			}},
		},
		{
			// Siblings are dialed at the peer listen address port.
			// An address that binds an arbitrary port leaves discovery with nothing to dial.
			name: "dns provider with no port to dial",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "127.0.0.1:0",
				AllowInsecure: true,
				Discovery: DiscoveryConfig{
					Provider: DiscoveryDNS,
					DNS:      DNSDiscoveryConfig{Name: "peers.svc.cluster.local"},
				},
			}}),
			want: validation.Errors{{
				Subject: "proxyAdmin.peer",
				Field:   "discovery.dns.port",
				Message: "is required",
			}},
		},
		{
			name: "static provider without addresses",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "127.0.0.1:9234",
				Discovery:     DiscoveryConfig{Provider: DiscoveryStatic},
			}}),
			want: validation.Errors{{
				Subject: "proxyAdmin.peer.discovery",
				Field:   "static.addresses",
				Message: "is required",
			}},
		},
		{
			// Layered configuration cannot delete keys.
			// The dns block outlives the switch away from it.
			// The selected provider must be the only one validated.
			name: "an unselected provider's block is inert",
			cfg: proxyAdmin(ProxyAdminConfig{Peer: &ProxyAdminPeerConfig{
				ListenAddress: "127.0.0.1:9234",
				Discovery: DiscoveryConfig{
					Provider: DiscoveryStatic,
					DNS:      DNSDiscoveryConfig{Name: "leftover.svc.cluster.local"},
					Static:   StaticDiscoveryConfig{Addresses: []string{"a:9234", "b:9234"}},
				},
			}}),
		},
		{
			name: "fully specified",
			cfg: proxyAdmin(ProxyAdminConfig{
				ListenAddress: "127.0.0.1:6061",
				Peer: &ProxyAdminPeerConfig{
					ListenAddress: "0.0.0.0:9234",
					TLS: &encryption.TLSConfig{
						CertificatePath: "/c", KeyPath: "/k",
						RemoteCAPath: "/ca", CAServerName: "peers",
					},
					Discovery: DiscoveryConfig{
						Provider: DiscoveryDNS,
						DNS:      DNSDiscoveryConfig{Name: "peers.svc.cluster.local"},
					},
				},
			}),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if c.want == nil {
				require.NoError(t, err)
				return
			}

			requireErrors(t, err, c.want)
		})
	}
}

// TestProxyAdminValidateFromYAML runs the whole path an operator hits: a config file that binds the
// operator listener to every interface, loaded and then validated.
func TestProxyAdminValidateFromYAML(t *testing.T) {
	path := writeYAML(t, `
clusterConnections:
  - name: cluster-a
proxyAdmin:
  listenAddress: "0.0.0.0:6061"
`)

	cfg, err := LoadConfig[S2SProxyConfig](path)
	require.NoError(t, err)

	requireErrors(t, cfg.Validate(), validation.Errors{{
		Subject: "proxyAdmin",
		Field:   "listenAddress",
		Message: notLoopback("0.0.0.0:6061", operatorRemedy),
	}})
}
