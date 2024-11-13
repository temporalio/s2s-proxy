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

type TransportType string

const (
	TCPTransport TransportType = "tcp"
	MuxTransport TransportType = "mux" // transport based on multiplexing over TCP
)

type MuxMode string

const (
	ClientMode MuxMode = "client" // client of the underly tcp connection in mux mode.
	ServerMode MuxMode = "server" // server of underly tcp connection in mux mode.
)

type (
	ConfigProvider interface {
		GetS2SProxyConfig() S2SProxyConfig
	}

	TCPServerSetting struct {
		// ListenAddress indicates the server address (Host:Port) for listening requests
		ListenAddress string                     `yaml:"listenAddress"`
		TLS           encryption.ServerTLSConfig `yaml:"tls"`
		// ExternalAddress is the externally reachable address of this server.
		ExternalAddress string `yaml:"externalAddress"`
	}

	TCPClientSetting struct {
		// ServerAddress indicates the address (Host:Port) for forwarding requests
		ServerAddress string                     `yaml:"serverAddress"`
		TLS           encryption.ClientTLSConfig `yaml:"tls"`
	}

	ProxyServerConfig struct {
		Type             TransportType `yaml:"type"`
		TCPServerSetting `yaml:"tcp"`
		MuxTransportName string `yaml:"mux"`
	}

	ProxyClientConfig struct {
		Type             TransportType `yaml:"type"`
		TCPClientSetting `yaml:"tcp"`
		MuxTransportName string `yaml:"mux"`
	}

	ProxyConfig struct {
		Name                     string                         `yaml:"name"`
		Server                   ProxyServerConfig              `yaml:"server"`
		Client                   ProxyClientConfig              `yaml:"client"`
		NamespaceNameTranslation NamespaceNameTranslationConfig `yaml:"namespaceNameTranslation"`
		ACLPolicy                *ACLPolicy                     `yaml:"aclPolicy"`
	}

	MuxTransportConfig struct {
		Name   string            `yaml:"name"`
		Mode   MuxMode           `yaml:"mode"`
		Client *TCPClientSetting `yaml:"client"`
		Server *TCPServerSetting `yaml:"server"`
	}

	S2SProxyConfig struct {
		Inbound       *ProxyConfig         `yaml:"inbound"`
		Outbound      *ProxyConfig         `yaml:"outbound"`
		MuxTransports []MuxTransportConfig `yaml:"mux"`
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

	AllowedMethods struct {
		AdminService []string `yaml:"adminService"`
	}

	ACLPolicy struct {
		AllowedMethods    AllowedMethods `yaml:"allowedMethods"`
		AllowedNamespaces []string       `yaml:"allowedNamespaces"`
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

func (cfg ProxyClientConfig) String() string {
	return marshalWithoutError(cfg)
}

func (cfg ProxyServerConfig) String() string {
	return marshalWithoutError(cfg)
}

type (
	MockConfigProvider struct {
		config S2SProxyConfig
	}
)

var (
	EmptyConfigProvider MockConfigProvider
)

func NewMockConfigProvider(config S2SProxyConfig) *MockConfigProvider {
	return &MockConfigProvider{config: config}
}

func (mc *MockConfigProvider) GetS2SProxyConfig() S2SProxyConfig {
	return mc.config
}
