package config

import (
	"github.com/temporalio/s2s-proxy/collect"
	"github.com/temporalio/s2s-proxy/encryption"
)

// Looking for examples? Check ./develop/sample-cluster-conn-config.yaml
type (
	ClusterConnConfig struct {
		Name                       string              `yaml:"name"`
		LocalServer                ClusterDefinition   `yaml:"localServer"`
		RemoteServer               ClusterDefinition   `yaml:"remoteServer"`
		NamespaceTranslation       StringTranslator    `yaml:"namespaceTranslation"`
		SearchAttributeTranslation SATranslationConfig `yaml:"searchAttributeTranslation"`
		OutboundHealthCheck        HealthCheckConfig   `yaml:"outboundHealthCheck"`
		InboundHealthCheck         HealthCheckConfig   `yaml:"inboundHealthCheck"`
		ShardCountConfig           ShardCountConfig    `yaml:"shardCount"`
	}
	StringTranslator struct {
		Mappings    []StringMapping `yaml:"mappings"`
		cachedBiMap collect.StaticBiMap[string, string]
	}
	StringMapping struct {
		LocalString  string `yaml:"localString"`
		RemoteString string `yaml:"remoteString"`
	}
	ClusterDefinition struct {
		Connection  TransportInfo `yaml:"connection"`
		ClusterInfo ClusterInfo   `yaml:"clusterInfo"`
		// ACLPolicy has a meaningful nil value: it separates no-policy from deny-all
		ACLPolicy *ACLPolicy `yaml:"aclPolicy"`
		// APIOverrides has a meaningful nil value: it separates override-to-zero and no-override
		APIOverrides *APIOverridesConfig `yaml:"apiOverrides"`
	}
	ClusterInfo struct {
		ServerVersion            string `yaml:"serverVersion"`
		ShardCount               int    `yaml:"shardCount"`
		FailoverVersionIncrement int    `yaml:"failoverVersionIncrement"`
		InitialFailoverVersion   int    `yaml:"initialFailoverVersion"`
	}
	ConnectionType string
	TransportInfo  struct {
		ConnectionType ConnectionType `yaml:"connectionType"`
		TcpClient      TCPTLSInfo     `yaml:"tcpClient"`
		TcpServer      TCPTLSInfo     `yaml:"tcpServer"`
		MuxCount       int            `yaml:"muxCount"`
		MuxAddressInfo TCPTLSInfo     `yaml:"muxAddressInfo"`
	}
	TCPTLSInfo struct {
		ConnectionString string               `yaml:"address"`
		TLSConfig        encryption.TLSConfig `yaml:"tls"`
	}
)

const (
	ConnTypeTCP       ConnectionType = "tcp"
	ConnTypeMuxServer ConnectionType = "mux-server"
	ConnTypeMuxClient ConnectionType = "mux-client"
)

func (config *StringTranslator) AsLocalToRemoteBiMap() (collect.StaticBiMap[string, string], error) {
	if config.cachedBiMap != nil {
		return config.cachedBiMap, nil
	}
	mapping, err := collect.NewStaticBiMap(func(yield func(string, string) bool) {
		for _, mapping := range config.Mappings {
			if !yield(mapping.LocalString, mapping.RemoteString) {
				return
			}
		}
	}, len(config.Mappings))
	if err != nil {
		return nil, err
	}
	config.cachedBiMap = mapping
	return config.cachedBiMap, nil
}
