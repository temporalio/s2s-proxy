package config

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		// ExternalAddress is the externally reachable address of this server.
		ExternalAddress string `yaml:"externalAddress"`
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
		Inbound  *ProxyConfig `yaml:"inbound"`
		Outbound *ProxyConfig `yaml:"outbound"`
	}

	NamespaceNameTranslationConfig struct {
		Mappings []NameMappingConfig `yaml:"mappings"`
	}

	NameMappingConfig struct {
		LocalName  string `yaml:"localName"`
		RemoteName string `yaml:"remoteName"`
	}

	cliConfigProvider struct {
		ctx       *cli.Context
		s2sConfig S2SProxyConfig
	}

	AllowedActionsConfig struct {
		AdminService []string `yaml:"adminservice"`
	}

	ACLConfig struct {
		AllowedActions    AllowedActionsConfig `yaml:"allowedActions"`
		AllowedNamespaces []string             `yaml:"allowedNamespaces"`
	}

	ACLPolicy struct {
		Migration ACLConfig `yaml:"migration"`
	}
)

func newConfigProvider(ctx *cli.Context) (ConfigProvider, error) {
	s2sConfig, err := LoadConfig[S2SProxyConfig](ctx.String(ConfigPathFlag))
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

func LoadConfig[T any](configFilePath string) (T, error) {
	var config T
	data, err := os.ReadFile(configFilePath)
	if err != nil {
		return config, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	err = decoder.Decode(&config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func WriteConfig[T any](config T, filePath string) error {
	// Marshal the struct to YAML
	data, err := yaml.Marshal(&config)
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

func marshalWithoutError(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%+v", v)
	}

	return string(data)
}

func (cfg ClientConfig) String() string {
	return marshalWithoutError(cfg)
}

func (cfg ServerConfig) String() string {
	return marshalWithoutError(cfg)
}
