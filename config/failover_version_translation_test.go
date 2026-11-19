package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func validFailoverVersionTranslation() FailoverVersionTranslationConfig {
	return FailoverVersionTranslationConfig{
		FVI: IntMapping{Local: 105, Remote: 100},
		IFV: IFVTranslationConfig{Mappings: []IntMapping{
			{Local: 1, Remote: 1},
			{Local: 2, Remote: 2},
			{Local: 105, Remote: 100},
		}},
	}
}

func TestFailoverVersionTranslationConfigValidate(t *testing.T) {
	t.Parallel()
	valid := validFailoverVersionTranslation()
	require.NoError(t, valid.Validate())

	cases := []struct {
		name string
		edit func(*FailoverVersionTranslationConfig)
		want string
	}{
		{"zero local fvi", func(c *FailoverVersionTranslationConfig) { c.FVI.Local = 0 }, "fvi.local"},
		{"negative remote fvi", func(c *FailoverVersionTranslationConfig) { c.FVI.Remote = -1 }, "fvi.remote"},
		{"empty mappings", func(c *FailoverVersionTranslationConfig) { c.IFV.Mappings = nil }, "ifv.mappings"},
		{"zero lane", func(c *FailoverVersionTranslationConfig) { c.IFV.Mappings[0].Local = 0 }, "ifv.mappings[0].local"},
		{"negative lane", func(c *FailoverVersionTranslationConfig) { c.IFV.Mappings[0].Local = -1 }, "ifv.mappings[0].local"},
		{"out of domain", func(c *FailoverVersionTranslationConfig) { c.IFV.Mappings[0].Remote = 101 }, "greater than failover version increment"},
		{"duplicate local", func(c *FailoverVersionTranslationConfig) { c.IFV.Mappings[1].Local = 1 }, "duplicates ifv.mappings[0].local"},
		{"duplicate remote", func(c *FailoverVersionTranslationConfig) { c.IFV.Mappings[1].Remote = 1 }, "duplicates ifv.mappings[0].remote"},
		{"order inversion", func(c *FailoverVersionTranslationConfig) { c.IFV.Mappings[0].Remote = 50 }, "must preserve local lane ordering"},
		{"int32 overflow", func(c *FailoverVersionTranslationConfig) {
			c.FVI.Local = math.MaxInt32 + 1
			c.IFV.Mappings[2].Local = math.MaxInt32 + 1
		}, "does not fit in int32"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := validFailoverVersionTranslation()
			tc.edit(&cfg)
			require.ErrorContains(t, cfg.Validate(), tc.want)
		})
	}
}

func TestFailoverVersionTranslationValidationDoesNotMutateConfig(t *testing.T) {
	t.Parallel()
	cfg := validFailoverVersionTranslation()
	want := cfg
	want.IFV.Mappings = append([]IntMapping(nil), cfg.IFV.Mappings...)
	require.NoError(t, cfg.Validate())
	require.Equal(t, want, cfg)

	missing := FailoverVersionTranslationConfig{}
	require.ErrorContains(t, missing.Validate(), "fvi.local")
}

func TestFailoverVersionTranslationAllowsUnsortedOrderPreservingMappings(t *testing.T) {
	t.Parallel()
	cfg := validFailoverVersionTranslation()
	cfg.IFV.Mappings[0], cfg.IFV.Mappings[2] = cfg.IFV.Mappings[2], cfg.IFV.Mappings[0]
	require.NoError(t, cfg.Validate())
}

func TestClusterConnRejectsLegacyAndSemanticFailoverTranslation(t *testing.T) {
	t.Parallel()
	translation := validFailoverVersionTranslation()
	cfg := ClusterConnConfig{
		FVITranslation:             IntMapping{Local: 105, Remote: 100},
		FailoverVersionTranslation: &translation,
	}
	require.ErrorContains(t, cfg.Validate(), "cannot be combined")
}

func TestFailoverVersionTranslationYAMLShape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
clusterConnections:
  - name: test
    failoverVersionTranslation:
      fvi:
        source: 105
        remote: 100
      ifv:
        mappings:
          - local: 1
            remote: 1
`), 0o600))
	_, err := LoadConfig[S2SProxyConfig](path)
	require.ErrorContains(t, err, "field source not found")
}
