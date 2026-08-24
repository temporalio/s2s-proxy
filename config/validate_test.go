package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/validation"
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
