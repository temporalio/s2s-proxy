package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func validNamespaceFailoverVersionRewrite() NamespaceFailoverVersionRewriteConfig {
	return NamespaceFailoverVersionRewriteConfig{
		Mode:       NamespaceFailoverVersionRewriteModeEnforce,
		Namespaces: []string{"migration"},
		Local: NamespaceFailoverVersionEndpoint{
			ClusterName: "self-hosted", InitialFailoverVersion: 1,
		},
		Remote: NamespaceFailoverVersionEndpoint{
			ClusterName: "cloud", InitialFailoverVersion: 2,
		},
	}
}

func testFVITranslation() IntMapping {
	// Caller-facing values are crossed: local is the remote native FVI.
	return IntMapping{Local: 10, Remote: 7}
}

func TestNamespaceFailoverVersionRewriteValidate(t *testing.T) {
	valid := validNamespaceFailoverVersionRewrite()
	require.NoError(t, valid.Validate(testFVITranslation()))
	valid.Mode = NamespaceFailoverVersionRewriteModeObserve
	require.NoError(t, valid.Validate(testFVITranslation()))

	tests := []struct {
		name string
		edit func(*NamespaceFailoverVersionRewriteConfig, *IntMapping)
		want string
	}{
		{"mode required", func(c *NamespaceFailoverVersionRewriteConfig, _ *IntMapping) { c.Mode = "" }, "mode must be"},
		{"namespace required", func(c *NamespaceFailoverVersionRewriteConfig, _ *IntMapping) { c.Namespaces = nil }, "namespaces"},
		{"namespace nonempty", func(c *NamespaceFailoverVersionRewriteConfig, _ *IntMapping) { c.Namespaces = []string{""} }, "empty"},
		{"namespace unique", func(c *NamespaceFailoverVersionRewriteConfig, _ *IntMapping) {
			c.Namespaces = []string{"migration", "migration"}
		}, "duplicate"},
		{"cluster required", func(c *NamespaceFailoverVersionRewriteConfig, _ *IntMapping) { c.Local.ClusterName = "" }, "clusterName"},
		{"cluster names distinct", func(c *NamespaceFailoverVersionRewriteConfig, _ *IntMapping) {
			c.Remote.ClusterName = c.Local.ClusterName
		}, "must differ"},
		{"positive fvi", func(_ *NamespaceFailoverVersionRewriteConfig, fvi *IntMapping) { fvi.Remote = 0 }, "failoverVersionIncrementTranslation"},
		{"ifv in both domains", func(c *NamespaceFailoverVersionRewriteConfig, _ *IntMapping) { c.Remote.InitialFailoverVersion = 8 }, "fit in both"},
		{"ifvs distinct", func(c *NamespaceFailoverVersionRewriteConfig, _ *IntMapping) { c.Remote.InitialFailoverVersion = 1 }, "must differ"},
		{"fvis differ", func(_ *NamespaceFailoverVersionRewriteConfig, fvi *IntMapping) { fvi.Local = fvi.Remote }, "must differ"},
		{"foreign lane required", func(c *NamespaceFailoverVersionRewriteConfig, fvi *IntMapping) {
			fvi.Remote = 2
			fvi.Local = 3
			c.Local.InitialFailoverVersion = 1
			c.Remote.InitialFailoverVersion = 2
		}, "no receiver-owned candidate"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := validNamespaceFailoverVersionRewrite()
			fvi := testFVITranslation()
			test.edit(&cfg, &fvi)
			require.ErrorContains(t, cfg.Validate(fvi), test.want)
		})
	}
}

func TestNamespaceFailoverVersionRewriteYAML(t *testing.T) {
	var cfg ClusterConnConfig
	require.NoError(t, yaml.Unmarshal([]byte(`
name: migration
failoverVersionIncrementTranslation:
  local: 10
  remote: 7
namespaceFailoverVersionRewrite:
  mode: enforce
  namespaces: [migration]
  local:
    clusterName: self-hosted
    initialFailoverVersion: 1
  remote:
    clusterName: cloud
    initialFailoverVersion: 2
`), &cfg))
	require.NoError(t, cfg.Validate())
	require.Equal(t, NamespaceFailoverVersionRewriteModeEnforce, cfg.NamespaceFailoverVersionRewrite.Mode)
}
