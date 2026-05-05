package config

import (
	"bytes"
	"os"

	"github.com/urfave/cli/v2"
	"gopkg.in/yaml.v3"

	"github.com/temporalio/s2s-proxy/collect"
)

const (
	ConfigPathFlag = "config"
	LogLevelFlag   = "level"

	DefaultPProfAddress          = "localhost:6060"
	DefaultLoggingThrottleMaxRPS = 10.0
)

type ShardCountMode string

const (
	ShardCountDefault ShardCountMode = ""
	ShardCountLCM     ShardCountMode = "lcm"
	ShardCountRouting ShardCountMode = "routing"
)

type HealthCheckProtocol string

const (
	HTTP HealthCheckProtocol = "http"
)

type (
	ConfigProvider interface {
		GetS2SProxyConfig() S2SProxyConfig
	}

	HealthCheckConfig struct {
		Protocol      HealthCheckProtocol `yaml:"protocol"`
		ListenAddress string              `yaml:"listenAddress"`
	}

	S2SProxyConfig struct {
		SearchAttributeTranslation SATranslationConfig      `yaml:"searchAttributeTranslation"`
		Metrics                    *MetricsConfig           `yaml:"metrics"`
		ProfilingConfig            *ProfilingConfig         `yaml:"profiling"`
		Logging                    LoggingConfig            `yaml:"logging"`
		LogConfigs                 map[string]LoggingConfig `yaml:"logConfigs"`
		ClusterConnections         []ClusterConnConfig      `yaml:"clusterConnections"`
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
		Disabled       bool    `yaml:"disabled"`
	}

	MemberlistConfig struct {
		// Enable distributed shard management using memberlist
		Enabled bool `yaml:"enabled"`
		// Node name for this proxy instance in the cluster
		NodeName string `yaml:"nodeName"`
		// Bind address for memberlist cluster communication
		BindAddr string `yaml:"bindAddr"`
		// Bind port for memberlist cluster communication
		BindPort int `yaml:"bindPort"`
		// List of existing cluster members to join
		JoinAddrs []string `yaml:"joinAddrs"`
		// Shard assignment strategy (deprecated - now uses actual ownership tracking)
		ShardStrategy string `yaml:"shardStrategy"`
		// Map of node names to their proxy service addresses for forwarding
		ProxyAddresses map[string]string `yaml:"proxyAddresses"`
		// Use TCP-only transport (disables UDP) for restricted networks
		TCPOnly bool `yaml:"tcpOnly"`
		// Disable TCP pings when using TCP-only mode
		DisableTCPPings bool `yaml:"disableTCPPings"`
		// Probe timeout for memberlist health checks
		ProbeTimeoutMs int `yaml:"probeTimeoutMs"`
		// Probe interval for memberlist health checks
		ProbeIntervalMs int `yaml:"probeIntervalMs"`
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
