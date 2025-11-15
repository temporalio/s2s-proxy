package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"

	"github.com/temporalio/s2s-proxy/collect"
	"github.com/temporalio/s2s-proxy/encryption"
)

const (
	ConfigPathFlag = "config"
	LogLevelFlag   = "level"

	DefaultPProfAddress          = "localhost:6060"
	DefaultLoggingThrottleMaxRPS = 10.0
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

type ShardCountMode string

const (
	ShardCountDefault ShardCountMode = ""
	ShardCountLCM     ShardCountMode = "lcm"
)

type HealthCheckProtocol string

const (
	HTTP HealthCheckProtocol = "http"
)

type (
	ConfigProvider interface {
		GetS2SProxyConfig() S2SProxyConfig
	}

	// ServerTLSConfig provides backwards compatibility with existing configurations. Prefer config.ClusterConnConfig instead.
	// TODO: This will be removed soon!
	ServerTLSConfig struct {
		CertificatePath   string `yaml:"certificatePath"`
		KeyPath           string `yaml:"keyPath"`
		ClientCAPath      string `yaml:"clientCAPath"`
		RequireClientAuth bool   `yaml:"requireClientAuth"`
	}

	// ClientTLSConfig provides backwards compatibility with existing configurations. Prefer config.ClusterConnConfig instead.
	// TODO: This will be removed soon!
	ClientTLSConfig struct {
		CertificatePath string `yaml:"certificatePath"`
		KeyPath         string `yaml:"keyPath"`
		ServerName      string `yaml:"serverName"`
		ServerCAPath    string `yaml:"serverCAPath"`
	}

	TCPServerSetting struct {
		// ListenAddress indicates the server address (Host:Port) for listening requests
		ListenAddress string          `yaml:"listenAddress"`
		TLS           ServerTLSConfig `yaml:"tls"`
		// ExternalAddress is the externally reachable address of this server.
		ExternalAddress string `yaml:"externalAddress"`
	}

	TCPClientSetting struct {
		// ServerAddress indicates the address (Host:Port) for forwarding requests
		ServerAddress string          `yaml:"serverAddress"`
		TLS           ClientTLSConfig `yaml:"tls"`
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

	AddOrUpdateRemoteClusterRequestOverrides struct {
		FrontendAddress string `yaml:"FrontendAddress"`
	}

	AddOrUpdateRemoteClusterOverride struct {
		Request AddOrUpdateRemoteClusterRequestOverrides `yaml:"request"`
	}

	AdminServiceOverrides struct {
		AddOrUpdateRemoteCluster *AddOrUpdateRemoteClusterOverride `yaml:"AddOrUpdateRemoteCluster"`
		DescribeCluster          *DescribeClusterOverride          `yaml:"DescribeCluster"`
	}

	APIOverridesConfig struct {
		AdminService AdminServiceOverrides `yaml:"adminService"`
	}

	ProxyConfig struct {
		Name         string              `yaml:"name"`
		Server       ProxyServerConfig   `yaml:"server"`
		Client       ProxyClientConfig   `yaml:"client"`
		ACLPolicy    *ACLPolicy          `yaml:"aclPolicy"`
		APIOverrides *APIOverridesConfig `yaml:"api_overrides"`
	}

	MuxTransportConfig struct {
		Name           string           `yaml:"name"`
		Mode           MuxMode          `yaml:"mode"`
		Client         TCPClientSetting `yaml:"client"`
		Server         TCPServerSetting `yaml:"server"`
		NumConnections int              `yaml:"num_connections"`
	}

	HealthCheckConfig struct {
		Protocol      HealthCheckProtocol `yaml:"protocol"`
		ListenAddress string              `yaml:"listenAddress"`
	}

	S2SProxyConfig struct {
		// TODO: Soon to be deprecated! Create an item in ClusterConnections instead
		Inbound *ProxyConfig `yaml:"inbound"`
		// TODO: Soon to be deprecated! Create an item in ClusterConnections instead
		Outbound *ProxyConfig `yaml:"outbound"`
		// TODO: Soon to be deprecated! Create an item in ClusterConnections instead
		MuxTransports []MuxTransportConfig `yaml:"mux"`
		// TODO: Soon to be deprecated! Create an item in ClusterConnections instead
		HealthCheck *HealthCheckConfig `yaml:"healthCheck"`
		// TODO: Soon to be deprecated! Create an item in ClusterConnections instead
		OutboundHealthCheck        *HealthCheckConfig    `yaml:"outboundHealthCheck"`
		NamespaceNameTranslation   NameTranslationConfig `yaml:"namespaceNameTranslation"`
		SearchAttributeTranslation SATranslationConfig   `yaml:"searchAttributeTranslation"`
		Metrics                    *MetricsConfig        `yaml:"metrics"`
		ProfilingConfig            ProfilingConfig       `yaml:"profiling"`
		Logging                    LoggingConfig         `yaml:"logging"`
		ClusterConnections         []ClusterConnConfig   `yaml:"clusterConnections"`
	}

	SATranslationConfig struct {
		NamespaceMappings []SANamespaceMapping `yaml:"namespaceMappings"`
		cachedBiMap       SearchAttributeTranslation
	}

	SANamespaceMapping struct {
		Name        string      `yaml:"name"`
		NamespaceId string      `yaml:"namespaceId"`
		Mappings    []SAMapping `yaml:"mappings"`
	}

	SAMapping struct {
		LocalName  string `yaml:"localFieldName"`
		RemoteName string `yaml:"remoteFieldName"`
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

	LoggingConfig struct {
		ThrottleMaxRPS float64 `yaml:"throttleMaxRPS"`
	}
)

func FromServerTLSConfig(cfg ServerTLSConfig) encryption.TLSConfig {
	return encryption.TLSConfig{
		CertificatePath:  cfg.CertificatePath,
		KeyPath:          cfg.KeyPath,
		RemoteCAPath:     cfg.ClientCAPath,
		ValidateClientCA: cfg.RequireClientAuth,
	}
}
func FromClientTLSConfig(cfg ClientTLSConfig) encryption.TLSConfig {
	return encryption.TLSConfig{
		CertificatePath: cfg.CertificatePath,
		KeyPath:         cfg.KeyPath,
		RemoteCAPath:    cfg.ServerCAPath,
		CAServerName:    cfg.ServerName,
	}
}

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

func (o MuxTransportConfig) GetLabelValues() []string {
	switch o.Mode {
	case ServerMode:
		return []string{o.Server.ListenAddress, string(o.Mode), o.Name}
	case ClientMode:
		return []string{o.Client.ServerAddress, string(o.Mode), o.Name}
	}
	return []string{"unknown", "unknown", o.Name}
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

func (n NameTranslationConfig) IsEnabled() bool {
	return len(n.Mappings) > 0
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

func (s *SATranslationConfig) IsEnabled() bool {
	return len(s.NamespaceMappings) > 0
}

// ToMaps returns request and response mappings.
func (s *SATranslationConfig) ToMaps(inBound bool) (map[string]map[string]string, map[string]map[string]string) {
	reqMap := make(map[string]map[string]string)
	respMap := make(map[string]map[string]string)
	for _, ns := range s.NamespaceMappings {
		reqMap[ns.NamespaceId] = make(map[string]string, len(ns.Mappings))
		respMap[ns.NamespaceId] = make(map[string]string, len(ns.Mappings))

		if inBound {
			// For inbound listener,
			//   - incoming requests from remote server are modifed to match local server
			//   - outgoing responses to local server are modified to match remote server
			for _, tr := range ns.Mappings {
				reqMap[ns.NamespaceId][tr.RemoteName] = tr.LocalName
				respMap[ns.NamespaceId][tr.LocalName] = tr.RemoteName
			}
		} else {
			// For outbound listener,
			//   - incoming requests from local server are modifed to match remote server
			//   - outgoing responses to remote server are modified to match local server
			for _, tr := range ns.Mappings {
				reqMap[ns.NamespaceId][tr.LocalName] = tr.RemoteName
				respMap[ns.NamespaceId][tr.RemoteName] = tr.LocalName
			}
		}
	}
	return reqMap, respMap
}

type SearchAttributeTranslation struct {
	inner    map[string]collect.StaticBiMap[string, string]
	inverted bool
}

func (s SearchAttributeTranslation) Inverse() SearchAttributeTranslation {
	return SearchAttributeTranslation{
		inner:    s.inner,
		inverted: !s.inverted,
	}
}
func (s SearchAttributeTranslation) withInvert(namespace string) (collect.StaticBiMap[string, string], bool) {
	m, found := s.inner[namespace]
	if !found {
		return nil, false
	}
	if s.inverted {
		return m.Inverse(), true
	}
	return m, true
}
func (s SearchAttributeTranslation) Get(namespace string, searchAttr string) string {
	if m, ok := s.withInvert(namespace); ok {
		return m.Get(searchAttr)
	}
	return ""
}
func (s SearchAttributeTranslation) GetExists(namespace string, searchAttr string) (string, bool) {
	if m, ok := s.withInvert(namespace); ok {
		return m.GetExists(searchAttr)
	}
	return "", false
}
func (s SearchAttributeTranslation) LenNamespaces() int {
	return len(s.inner)
}
func (s SearchAttributeTranslation) Len(namespace string) int {
	if m, ok := s.withInvert(namespace); ok {
		return m.Len()
	}
	return 0
}
func (s SearchAttributeTranslation) FlattenMaps() map[string]map[string]string {
	raw := make(map[string]map[string]string, len(s.inner))
	for ns, mappings := range s.inner {
		if s.inverted {
			mappings = mappings.Inverse()
		}
		raw[ns] = mappings.AsMap()
	}
	return raw
}

// AsLocalToRemoteSATranslation converts the flat list of namespace + local/remote pairs into a map of BiMaps, with local->remote
// as the direction returned. The remote->local mapping can be accessed with saTranslator[namespaceId].Inverse()
func (s *SATranslationConfig) AsLocalToRemoteSATranslation() (SearchAttributeTranslation, error) {
	if s.cachedBiMap.inner != nil {
		return s.cachedBiMap, nil
	}
	saTranslation := SearchAttributeTranslation{
		inner: make(map[string]collect.StaticBiMap[string, string], len(s.NamespaceMappings)),
	}
	for _, mapping := range s.NamespaceMappings {
		var err error
		saTranslation.inner[mapping.NamespaceId], err = collect.NewStaticBiMap(func(yield func(string, string) bool) {
			for _, attrPair := range mapping.Mappings {
				if !yield(attrPair.LocalName, attrPair.RemoteName) {
					return
				}
			}
		}, len(mapping.Mappings))
		if err != nil {
			return SearchAttributeTranslation{}, err
		}
	}
	s.cachedBiMap = saTranslation
	return saTranslation, nil
}

func (l LoggingConfig) GetThrottleMaxRPS() float64 {
	if l.ThrottleMaxRPS > 0 {
		return l.ThrottleMaxRPS
	}
	return DefaultLoggingThrottleMaxRPS
}
