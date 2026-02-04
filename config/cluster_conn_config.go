package config

import (
	"github.com/temporalio/s2s-proxy/collect"
	"github.com/temporalio/s2s-proxy/encryption"
)

// Looking for examples? Check ./develop/sample-cluster-conn-config.yaml
type (
	ClusterConnConfig struct {
		Name                       string              `yaml:"name"`
		Local                      ClusterDefinition   `yaml:"local"`
		Remote                     ClusterDefinition   `yaml:"remote"`
		ReplicationEndpoint        string              `yaml:"replicationEndpoint"`
		FVITranslation             IntMapping          `yaml:"failoverVersionIncrementTranslation"`
		ACLPolicy                  *ACLPolicy          `yaml:"aclPolicy"`
		NamespaceTranslation       StringTranslator    `yaml:"namespaceTranslation"`
		SearchAttributeTranslation SATranslationConfig `yaml:"searchAttributeTranslation"`
		RemoteClusterHealthCheck   HealthCheckConfig   `yaml:"remoteClusterHealthCheck"`
		LocalClusterHealthCheck    HealthCheckConfig   `yaml:"localClusterHealthCheck"`
		ShardCountConfig           ShardCountConfig    `yaml:"shardCount"`
		MemberlistConfig           *MemberlistConfig   `yaml:"memberlist"`
	}
	StringTranslator struct {
		Mappings    []StringMapping `yaml:"mappings"`
		cachedBiMap collect.StaticBiMap[string, string]
	}
	StringMapping struct {
		Local  string `yaml:"local"`
		Remote string `yaml:"remote"`
	}
	IntMapping struct {
		Local  int64 `yaml:"local"`
		Remote int64 `yaml:"remote"`
	}
	ConnectionType    string
	ClusterDefinition struct {
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

	ShardCountConfig struct {
		Mode             ShardCountMode `yaml:"mode"`
		LocalShardCount  int32          `yaml:"localShardCount"`
		RemoteShardCount int32          `yaml:"remoteShardCount"`
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
			if !yield(mapping.Local, mapping.Remote) {
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
