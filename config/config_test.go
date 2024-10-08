package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadS2SConfig(t *testing.T) {
	currPath, err := os.Getwd()
	assert.NoError(t, err)
	samplePath := filepath.Join("..", "develop", "sample-config.yaml")

	fmt.Println(currPath)
	s2sConfig, err := LoadConfig[S2SProxyConfig](samplePath)
	assert.NoError(t, err)
	assert.Equal(t, "inbound-proxy", s2sConfig.Inbound.Name)
	assert.Equal(t, "outbound-proxy", s2sConfig.Outbound.Name)
	assert.Equal(t, []NameMappingConfig{
		{
			LocalName:  "example",
			RemoteName: "example.cloud",
		},
	}, s2sConfig.Outbound.NamespaceNameTranslation.Mappings)
}

func TestLoadACLPolicy(t *testing.T) {
	currPath, err := os.Getwd()
	assert.NoError(t, err)
	samplePath := filepath.Join("..", "develop", "sample-acl-config.yaml")

	fmt.Println(currPath)
	aclConfig, err := LoadConfig[ACLPolicy](samplePath)
	assert.NoError(t, err)
	assert.Greater(t, len(aclConfig.Migration.AllowedMethods.AdminService), 0)
	assert.Equal(t, []string{"namespace1", "namespace2"}, aclConfig.Migration.AllowedNamespaces)
}
