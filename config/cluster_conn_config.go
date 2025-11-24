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

	// ClusterInfo is not used right now. In the future it will contain detected details from the Temporal cluster
	ClusterInfo struct {
		ServerVersion            string `yaml:"serverVersion"`
		ShardCount               int    `yaml:"shardCount"`
		FailoverVersionIncrement int    `yaml:"failoverVersionIncrement"`
		InitialFailoverVersion   int    `yaml:"initialFailoverVersion"`
	}
	ConnectionType string

	// TransportInfo is a union of the tcp and mux config objects. Valid configs are one of:
	// ConnectionType=tcp, TcpClient, TcpServer
	// ConnectionType=mux-client, MuxAddressInfo, MuxCount
	// ConnectionType=mux-server, MuxAddressInfo, MuxCount
	TransportInfo struct {
		ConnectionType ConnectionType `yaml:"connectionType"`
		// TcpClient is the client used to call this server. Used only for ConnectionType=tcp
		TcpClient TCPTLSInfo `yaml:"tcpClient"`
		// TcpServer is the server responsible for receiving calls from the remote server. Used only for ConnectionType=tcp
		TcpServer TCPTLSInfo `yaml:"tcpServer"`
		MuxCount  int        `yaml:"muxCount"`
		// MuxAddressInfo is the mux address for the connection. On mux-server, this is the listen address, usually "0.0.0.0:<port>"
		// On mux-client, this is the remote address+port, e.g. "<remote-ip>:<port>"
		MuxAddressInfo TCPTLSInfo `yaml:"muxAddressInfo"`
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
