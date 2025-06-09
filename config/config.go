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
	LogLevelFlag   = "level"

	DefaultPProfAddress = "localhost:6060"
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

type HealthCheckProtocol string

const (
	HTTP HealthCheckProtocol = "http"
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

	DescribeClusterResponseOverrides struct {
		FailoverVersionIncrement *int64 `yaml:"failover_version_increment,omitempty"`
	}

	DescribeClusterOverride struct {
		Response DescribeClusterResponseOverrides `yaml:"response"`
	}

	AdminServiceOverrides struct {
		DescribeCluster *DescribeClusterOverride `yaml:"DescribeCluster"`
	}

	APIOverridesConfig struct {
		AdminSerivce AdminServiceOverrides `yaml:"adminService"`
	}

	ProxyConfig struct {
		Name         string              `yaml:"name"`
		Server       ProxyServerConfig   `yaml:"server"`
		Client       ProxyClientConfig   `yaml:"client"`
		ACLPolicy    *ACLPolicy          `yaml:"aclPolicy"`
		APIOverrides *APIOverridesConfig `yaml:"api_overrides"`
	}

	MuxTransportConfig struct {
		Name   string           `yaml:"name"`
		Mode   MuxMode          `yaml:"mode"`
		Client TCPClientSetting `yaml:"client"`
		Server TCPServerSetting `yaml:"server"`
	}

	HealthCheckConfig struct {
		Protocol      HealthCheckProtocol `yaml:"protocol"`
		ListenAddress string              `yaml:"listenAddress"`
	}

	S2SProxyConfig struct {
		Inbound                  *ProxyConfig          `yaml:"inbound"`
		Outbound                 *ProxyConfig          `yaml:"outbound"`
		MuxTransports            []MuxTransportConfig  `yaml:"mux"`
		HealthCheck              *HealthCheckConfig    `yaml:"healthCheck"`
		NamespaceNameTranslation NameTranslationConfig `yaml:"namespaceNameTranslation"`
		ClusterNameTranslation   NameTranslationConfig `yaml:"clusterNameTranslation"`
		Metrics                  *MetricsConfig        `yaml:"metrics"`
		ProfilingConfig          ProfilingConfig       `yaml:"profiling"`
	}

	ProfilingConfig struct {
		PProfHTTPAddress string `yaml:"pprofAddress"`
	}

	NameTranslationConfig struct {
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

	PrometheusConfig struct {
		ListenAddress string `yaml:"listenAddress"`
		Framework     string `yaml:"framework"`
	}

	MetricsConfig struct {
		Prometheus PrometheusConfig `yaml:"prometheus"`
	}
)

func (c ProxyClientConfig) IsMux() bool {
	return c.Type == MuxTransport
}

func (c ProxyClientConfig) IsTCP() bool {
	return c.Type == TCPTransport
}

func (c *ProxyClientConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set default
	c.Type = TCPTransport

	// Alias to avoid infinite recursion
	type plain ProxyClientConfig
	return unmarshal((*plain)(c))
}

func (c ProxyServerConfig) IsMux() bool {
	return c.Type == MuxTransport
}

func (c ProxyServerConfig) IsTCP() bool {
	return c.Type == TCPTransport
}

func (c *ProxyServerConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// Set default
	c.Type = TCPTransport

	// Alias to avoid infinite recursion
	type plain ProxyServerConfig
	return unmarshal((*plain)(c))
}

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

func (o ProxyClientConfig) String() string {
	return marshalWithoutError(o)
}

func (o ProxyServerConfig) String() string {
	return marshalWithoutError(o)
}

func (o MuxTransportConfig) String() string {
	return marshalWithoutError(o)
}

func (o TCPServerSetting) String() string {
	return marshalWithoutError(o)
}

func (o TCPClientSetting) String() string {
	return marshalWithoutError(o)
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

// ToMaps returns request and response mappings.
func (n NameTranslationConfig) ToMaps(inBound bool) (map[string]string, map[string]string) {
	reqMap := make(map[string]string)
	respMap := make(map[string]string)
	if inBound {
		// For inbound listener,
		//   - incoming requests from remote server are modifed to match local server
		//   - outgoing responses to local server are modified to match remote server
		for _, tr := range n.Mappings {
			reqMap[tr.RemoteName] = tr.LocalName
			respMap[tr.LocalName] = tr.RemoteName
		}
	} else {
		// For outbound listener,
		//   - incoming requests from local server are modifed to match remote server
		//   - outgoing responses to remote server are modified to match local server
		for _, tr := range n.Mappings {
			reqMap[tr.LocalName] = tr.RemoteName
			respMap[tr.RemoteName] = tr.LocalName
		}
	}
	return reqMap, respMap
}

func (c *ProfilingConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	if len(c.PProfHTTPAddress) == 0 {
		c.PProfHTTPAddress = DefaultPProfAddress
	}

	// Alias to avoid infinite recursion
	type plain ProfilingConfig
	return unmarshal((*plain)(c))
}
