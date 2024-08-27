package config

import (
	"bytes"
	"os"

	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"
)

const (
	ConfigPathFlag = "config"
)

type (
	ConfigProvider interface {
		GetS2SProxyConfig() S2SProxyConfig
	}

	ServerConfig struct {
		// ListenAddress indicates the server address (Host:Port) for listening requests
		ListenAddress string                     `yaml:"listenAddress"`
		TLS           encryption.ServerTLSConfig `yaml:"tls"`
	}

	ClientConfig struct {
		// ForwardAddress indicates the address (Host:Port) for forwarding requests
		ForwardAddress string                     `yaml:"forwardAddress"`
		TLS            encryption.ClientTLSConfig `yaml:"tls"`
	}

	ProxyConfig struct {
		Name                     string                         `yaml:"name"`
		Server                   ServerConfig                   `yaml:"server"`
		Client                   ClientConfig                   `yaml:"client"`
		NamespaceNameTranslation NamespaceNameTranslationConfig `yaml:"namespaceNameTranslation"`
	}

	S2SProxyConfig struct {
		Inbound  ProxyConfig `yaml:"inbound"`
		Outbound ProxyConfig `yaml:"outbound"`
	}

	NamespaceNameTranslationConfig struct {
		Mappings                    []NameMappingConfig `yaml:"mappings"`
		ReflectionRecursionMaxDepth int                 `yaml:"reflectionRecursionMaxDepth"`
	}

	NameMappingConfig struct {
		LocalName  string `yaml:"localName"`
		RemoteName string `yaml:"remoteName"`
	}

	cliConfigProvider struct {
		ctx       *cli.Context
		s2sConfig S2SProxyConfig
	}
)

func newConfigProvider(ctx *cli.Context) (ConfigProvider, error) {
	s2sConfig, err := LoadConfig(ctx.String(ConfigPathFlag))
	if err != nil {
		return nil, err
	}

	return &cliConfigProvider{
		ctx:       ctx,
		s2sConfig: s2sConfig,
	}, nil
}

func (c *cliConfigProvider) GetS2SProxyConfig() S2SProxyConfig {
	return c.s2sConfig
}

func LoadConfig(configFilePath string) (S2SProxyConfig, error) {
	var proxyConfig S2SProxyConfig
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return proxyConfig, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	err = decoder.Decode(&proxyConfig)
	if err != nil {
		return proxyConfig, err
	}

	return proxyConfig, nil
}

func WriteConfig(s2sConfig S2SProxyConfig, filePath string) error {
	// Marshal the struct to YAML
	data, err := yaml.Marshal(&s2sConfig)
	if err != nil {
		return err
	}

	// Write the YAML to a file
	err = os.WriteFile(filePath, data, 0644)
	if err != nil {
		return err
	}

	return nil
}
