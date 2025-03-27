package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadS2SConfig(t *testing.T) {
	samplePath := filepath.Join("..", "develop", "sample-config.yaml")

	s2sConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	assert.NoError(t, err)
	assert.Equal(t, "inbound-proxy", s2sConfig.Inbound.Name)
	assert.Equal(t, "outbound-proxy", s2sConfig.Outbound.Name)
	assert.Equal(t, []NameMappingConfig{
		{
			LocalName:  "example",
			RemoteName: "example.cloud",
		},
	}, s2sConfig.NamespaceNameTranslation.Mappings)

	aclConfig := s2sConfig.Inbound.ACLPolicy
	assert.NotEmpty(t, aclConfig)
	assert.Greater(t, len(aclConfig.AllowedMethods.AdminService), 0)
	assert.Equal(t, []string{"namespace1", "namespace2"}, aclConfig.AllowedNamespaces)
	assert.Equal(t, HTTP, s2sConfig.HealthCheck.Protocol)
	assert.Equal(t, int64(100), *s2sConfig.Inbound.APIOverrides.DescribeCluster.Response.FailoverVersionIncrement)
}

func TestLoadS2SConfigMux(t *testing.T) {
	configFiles := []string{
		"cluster-a-mux-client-proxy.yaml",
		"cluster-b-mux-server-proxy.yaml",
	}

	for _, file := range configFiles {
		samplePath := filepath.Join("..", "develop", "config", file)
		_, err := LoadConfig[S2SProxyConfig](samplePath)
		assert.NoError(t, err)
	}
}
