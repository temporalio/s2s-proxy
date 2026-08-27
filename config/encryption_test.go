package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"github.com/temporalio/temporal-proxy/pkg/validation"
)

func TestKeyPolicyValidate(t *testing.T) {
	cases := []struct {
		name   string
		policy KeyPolicy
		want   validation.Errors
	}{
		{
			name:   "valid policy",
			policy: validPolicy(),
		},
		{
			name: "zero renewBefore is allowed",
			policy: KeyPolicy{
				URI:      "gcpkms://key",
				Duration: time.Hour,
			},
		},
		{
			name: "scheme match is case insensitive",
			policy: KeyPolicy{
				URI:      "AWSKMS://key",
				Duration: time.Hour,
			},
		},
		{
			name: "decrypt URIs are all valid",
			policy: KeyPolicy{
				URI:         "awskms://key",
				DecryptURIs: []string{"gcpkms://old", "testing://older"},
				Duration:    time.Hour,
			},
		},
		{
			name: "unknown scheme",
			policy: KeyPolicy{
				URI:      "vault://key",
				Duration: time.Hour,
			},
			want: validation.Errors{
				{Field: "uri", Message: invalidURI("vault://key")},
			},
		},
		{
			name: "unparseable URI",
			policy: KeyPolicy{
				URI:      "::",
				Duration: time.Hour,
			},
			want: validation.Errors{
				{Field: "uri", Message: `is not a valid URI: parse "::": missing protocol scheme`},
			},
		},
		{
			name:   "zero URI has no scheme",
			policy: KeyPolicy{Duration: time.Hour},
			want: validation.Errors{
				{Field: "uri", Message: invalidURI("")},
			},
		},
		{
			name: "decrypt URI reported by index",
			policy: KeyPolicy{
				URI:         "awskms://key",
				DecryptURIs: []string{"gcpkms://ok", "vault://nope"},
				Duration:    time.Hour,
			},
			want: validation.Errors{
				{Subject: "decryptURIs[1]", Message: invalidURI("vault://nope")},
			},
		},
		{
			name: "zero duration also fails the renewBefore bound",
			policy: KeyPolicy{
				URI: "awskms://key",
			},
			want: validation.Errors{
				{Field: "duration", Message: "not greater than 0s"},
				{Field: "renewBefore", Message: "not less than 0s"},
			},
		},
		{
			// A bogus duration still forms the upper bound for renewBefore, so
			// both fields report. That is noisy but honest: nothing is dropped.
			name: "negative duration drags renewBefore down with it",
			policy: KeyPolicy{
				URI:      "awskms://key",
				Duration: -time.Hour,
			},
			want: validation.Errors{
				{Field: "duration", Message: "not greater than 0s"},
				{Field: "renewBefore", Message: "not less than -1h0m0s"},
			},
		},
		{
			name: "negative renewBefore",
			policy: KeyPolicy{
				URI:         "awskms://key",
				Duration:    time.Hour,
				RenewBefore: -time.Minute,
			},
			want: validation.Errors{
				{Field: "renewBefore", Message: "not greater than or equal to 0s"},
			},
		},
		{
			name: "renewBefore equal to duration",
			policy: KeyPolicy{
				URI:         "awskms://key",
				Duration:    time.Hour,
				RenewBefore: time.Hour,
			},
			want: validation.Errors{
				{Field: "renewBefore", Message: "not less than 1h0m0s"},
			},
		},
		{
			name: "renewBefore longer than duration",
			policy: KeyPolicy{
				URI:         "awskms://key",
				Duration:    time.Hour,
				RenewBefore: 2 * time.Hour,
			},
			want: validation.Errors{
				{Field: "renewBefore", Message: "not less than 1h0m0s"},
			},
		},
		{
			name: "every failure is reported",
			policy: KeyPolicy{
				URI:         "vault://key",
				DecryptURIs: []string{"vault://old"},
				Duration:    -time.Hour,
				RenewBefore: -time.Minute,
			},
			want: validation.Errors{
				{Field: "uri", Message: invalidURI("vault://key")},
				{Subject: "decryptURIs[0]", Message: invalidURI("vault://old")},
				{Field: "duration", Message: "not greater than 0s"},
				{Field: "renewBefore", Message: "not greater than or equal to 0s"},
				{Field: "renewBefore", Message: "not less than -1h0m0s"},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.policy.Validate()
			if c.want == nil {
				require.NoError(t, err)
				return
			}

			requireErrors(t, err, c.want)
		})
	}
}

func TestKeyPolicyValidateSchemes(t *testing.T) {
	for _, scheme := range crypto.DefaultSchemes() {
		t.Run(scheme, func(t *testing.T) {
			policy := KeyPolicy{
				URI:      scheme + "://key",
				Duration: time.Hour,
			}

			require.NoError(t, policy.Validate())
		})
	}
}

func TestEncryptionConfigValidate(t *testing.T) {
	badPolicy := KeyPolicy{URI: "vault://key"}
	badPolicyErrs := func(subject string) validation.Errors {
		return validation.Errors{
			{Subject: subject, Field: "uri", Message: invalidURI("vault://key")},
			{Subject: subject, Field: "duration", Message: "not greater than 0s"},
			{Subject: subject, Field: "renewBefore", Message: "not less than 0s"},
		}
	}

	cases := []struct {
		name string
		cfg  EncryptionConfig
		want validation.Errors
	}{
		{
			name: "zero value is valid",
		},
		{
			name: "disabled needs no default",
			cfg:  EncryptionConfig{CacheSize: 100},
		},
		{
			name: "enabled with a valid default",
			cfg: func() EncryptionConfig {
				p := validPolicy()
				return EncryptionConfig{Enabled: true, CacheSize: 10, Default: &p}
			}(),
		},
		{
			name: "enabled requires a default",
			cfg:  EncryptionConfig{Enabled: true},
			want: validation.Errors{
				{Field: "default", Message: "is required"},
			},
		},
		{
			name: "negative cache size",
			cfg:  EncryptionConfig{CacheSize: -1},
			want: validation.Errors{
				{Field: "cacheSize", Message: "not greater than or equal to 0"},
			},
		},
		{
			name: "default is validated even while disabled",
			cfg:  EncryptionConfig{Default: &badPolicy},
			want: badPolicyErrs("default"),
		},
		{
			name: "overrides are validated even while disabled",
			cfg: EncryptionConfig{
				Overrides: map[string]KeyPolicy{"ns1": badPolicy},
			},
			want: badPolicyErrs("overrides[ns1]"),
		},
		{
			name: "valid overrides pass",
			cfg: EncryptionConfig{
				Overrides: map[string]KeyPolicy{"ns1": validPolicy(), "ns2": validPolicy()},
			},
		},
		{
			name: "override errors are sorted by namespace",
			cfg: EncryptionConfig{
				Overrides: map[string]KeyPolicy{"zeta": badPolicy, "alpha": badPolicy},
			},
			want: append(badPolicyErrs("overrides[alpha]"), badPolicyErrs("overrides[zeta]")...),
		},
		{
			name: "empty namespace key is rejected",
			cfg: EncryptionConfig{
				Overrides: map[string]KeyPolicy{"": validPolicy()},
			},
			want: validation.Errors{
				{Field: "overrides[]", Message: "is required"},
			},
		},
		{
			name: "failures across every rule are reported",
			cfg: EncryptionConfig{
				Enabled:   true,
				CacheSize: -5,
				Overrides: map[string]KeyPolicy{"ns1": badPolicy},
			},
			want: append(
				validation.Errors{
					{Field: "cacheSize", Message: "not greater than or equal to 0"},
					{Field: "default", Message: "is required"},
				},
				badPolicyErrs("overrides[ns1]")...,
			),
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

func TestEncryptionConfigFromYAML(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    EncryptionConfig
		wantErr string
	}{
		{
			name: "every field set",
			body: `
enabled: true
cacheSize: 256
default:
  uri: awskms://alias/primary
  decryptURIs:
    - awskms://alias/retired
    - gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/old
  duration: 720h
  renewBefore: 24h
overrides:
  tenant-a:
    uri: gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/a
    duration: 24h
    renewBefore: 1h
`,
			want: EncryptionConfig{
				Enabled:   true,
				CacheSize: 256,
				Default: &KeyPolicy{
					URI: "awskms://alias/primary",
					DecryptURIs: []string{
						"awskms://alias/retired",
						"gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/old",
					},
					Duration:    720 * time.Hour,
					RenewBefore: 24 * time.Hour,
				},
				Overrides: map[string]KeyPolicy{
					"tenant-a": {
						URI:         "gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/a",
						Duration:    24 * time.Hour,
						RenewBefore: time.Hour,
					},
				},
			},
		},
		{
			name: "omitted fields keep their zero values",
			body: "enabled: true\n",
			want: EncryptionConfig{Enabled: true},
		},
		{
			name: "durations take any Go suffix",
			body: `
default:
  uri: testing://key
  duration: 90m
  renewBefore: 30s
`,
			want: EncryptionConfig{
				Default: &KeyPolicy{
					URI:         "testing://key",
					Duration:    90 * time.Minute,
					RenewBefore: 30 * time.Second,
				},
			},
		},
		{
			name:    "unknown field is rejected",
			body:    "enabled: true\ncacheSiz: 10\n",
			wantErr: "field cacheSiz not found",
		},
		{
			name: "bare integer duration is rejected",
			body: `
default:
  uri: testing://key
  duration: 3600000000000
`,
			wantErr: "cannot unmarshal !!int",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := LoadConfig[EncryptionConfig](writeYAML(t, c.body))
			if c.wantErr != "" {
				require.ErrorContains(t, err, c.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, c.want, got)
		})
	}
}

func TestEncryptionConfigFromClusterConnection(t *testing.T) {
	path := writeYAML(t, `
clusterConnections:
  - name: cluster-a
    encryption:
      enabled: true
      cacheSize: 64
      default:
        uri: awskms://alias/primary
        duration: 24h
        renewBefore: 1h
`)

	cfg, err := LoadConfig[S2SProxyConfig](path)
	require.NoError(t, err)
	require.Len(t, cfg.ClusterConnections, 1)

	got := cfg.ClusterConnections[0].EncryptionConfig
	require.Equal(t, EncryptionConfig{
		Enabled:   true,
		CacheSize: 64,
		Default: &KeyPolicy{
			URI:         "awskms://alias/primary",
			Duration:    24 * time.Hour,
			RenewBefore: time.Hour,
		},
	}, got)
	require.NoError(t, got.Validate())
}

func TestEncryptionConfigYAMLRoundTrip(t *testing.T) {
	original := EncryptionConfig{
		Enabled:   true,
		CacheSize: 128,
		Default: &KeyPolicy{
			URI:         "awskms://alias/primary",
			DecryptURIs: []string{"gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/old"},
			Duration:    720 * time.Hour,
			RenewBefore: 24 * time.Hour,
		},
		Overrides: map[string]KeyPolicy{
			"tenant-a": {
				URI:         "testing://key",
				DecryptURIs: []string{"testing://retired"},
				Duration:    time.Hour,
			},
		},
	}

	path := filepath.Join(t.TempDir(), "yaml")
	require.NoError(t, WriteConfig(original, path))

	loaded, err := LoadConfig[EncryptionConfig](path)
	require.NoError(t, err)
	require.Equal(t, original, loaded)
	require.NoError(t, loaded.Validate())
}

func TestEncryptionConfigYAMLOmitsEmptyCollections(t *testing.T) {
	original := EncryptionConfig{
		Default: &KeyPolicy{URI: "testing://key", Duration: time.Hour},
	}
	require.Nil(t, original.Default.DecryptURIs)
	require.Nil(t, original.Overrides)

	path := filepath.Join(t.TempDir(), "yaml")
	require.NoError(t, WriteConfig(original, path))

	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NotContains(t, string(body), "decryptURIs")
	require.NotContains(t, string(body), "overrides")

	loaded, err := LoadConfig[EncryptionConfig](path)
	require.NoError(t, err)
	require.Equal(t, original, loaded)
}

func TestEncryptionConfigYAMLThenValidate(t *testing.T) {
	path := writeYAML(t, `
enabled: true
default:
  uri: awskms://alias/primary
  duration: 24h
overrides:
  tenant-a:
    uri: vault://alias/typo
    duration: 24h
`)

	cfg, err := LoadConfig[EncryptionConfig](path)
	require.NoError(t, err)

	requireErrors(t, cfg.Validate(), validation.Errors{
		{Subject: "overrides[tenant-a]", Field: "uri", Message: invalidURI("vault://alias/typo")},
	})
}

func invalidURI(raw string) string {
	return "invalid key URI: " + raw + ", valid schemes: [" + strings.Join(crypto.DefaultSchemes(), ",") + "]"
}

func requireErrors(t *testing.T, err error, want validation.Errors) {
	t.Helper()

	var got validation.Errors
	require.ErrorAs(t, err, &got)
	require.Equal(t, want, got)
}

func validPolicy() KeyPolicy {
	return KeyPolicy{
		URI:         "awskms://key",
		Duration:    24 * time.Hour,
		RenewBefore: time.Hour,
	}
}

func writeYAML(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))

	return path
}
