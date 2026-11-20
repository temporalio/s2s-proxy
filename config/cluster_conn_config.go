package config

import (
	"errors"

	"github.com/temporalio/temporal-proxy/pkg/validation"

	"github.com/temporalio/s2s-proxy/collect"
	"github.com/temporalio/s2s-proxy/encryption"
)

// Looking for examples? Check ./develop/sample-cluster-conn-config.yaml
type (
	ClusterConnConfig struct {
		Name                         string              `yaml:"name"`
		Local                        ClusterDefinition   `yaml:"local"`
		Remote                       ClusterDefinition   `yaml:"remote"`
		ReplicationEndpoint          string              `yaml:"replicationEndpoint"`
		FVITranslation               IntMapping          `yaml:"failoverVersionIncrementTranslation"`
		ACLPolicy                    *ACLPolicy          `yaml:"aclPolicy"`
		NamespaceTranslation         StringTranslator    `yaml:"namespaceTranslation"`
		SearchAttributeTranslation   SATranslationConfig `yaml:"searchAttributeTranslation"`
		CustomSearchAttributeAliases CustomSAAliasConfig `yaml:"customSearchAttributeAliases"`
		RemoteClusterHealthCheck     HealthCheckConfig   `yaml:"remoteClusterHealthCheck"`
		LocalClusterHealthCheck      HealthCheckConfig   `yaml:"localClusterHealthCheck"`
		ShardCountConfig             ShardCountConfig    `yaml:"shardCount"`
		MemberlistConfig             *MemberlistConfig   `yaml:"memberlist"`
		EncryptionConfig             EncryptionConfig    `yaml:"encryption"`
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

	ConnectionType string

	ClusterDefinition struct {
		ConnectionType ConnectionType `yaml:"connectionType"`
		TcpClient      TCPTLSInfo     `yaml:"tcpClient"`
		TcpServer      TCPTLSInfo     `yaml:"tcpServer"`
		MuxCount       int            `yaml:"muxCount"`
		MuxPools       *MuxPoolConfig `yaml:"muxPools,omitempty"`
		MuxAddressInfo TCPTLSInfo     `yaml:"muxAddressInfo"`
	}

	// MuxPoolConfig reserves independent physical mux sessions for general and
	// replication traffic. Both pools are required when the block is present.
	MuxPoolConfig struct {
		GeneralCount     int `yaml:"generalCount"`
		ReplicationCount int `yaml:"replicationCount"`
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

// Validate reports problems in nested transport and encryption configuration.
// The remaining checks NewProxy makes inline could move here over time.
func (c *ClusterConnConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Nested("local", &c.Local),
		validation.Nested("remote", &c.Remote),
		validation.Nested("encryption", &c.EncryptionConfig),
	)
}

func (c *ClusterDefinition) Validate() error {
	rules := []validation.Rule{
		validation.WhenNested(func() bool { return c.MuxPools != nil }, "muxPools", c.MuxPools),
	}
	if c.MuxPools != nil && c.ConnectionType != ConnTypeMuxClient && c.ConnectionType != ConnTypeMuxServer {
		rules = append(rules, validation.Field("muxPools", c.MuxPools, func(*MuxPoolConfig) error {
			return errors.New("is only supported for mux-client and mux-server connections")
		}))
	}
	if c.MuxPools != nil && c.MuxCount != 0 {
		rules = append(rules, validation.Field("muxCount", c.MuxCount, func(int) error {
			return errors.New("cannot be set together with muxPools")
		}))
	}
	return validation.Validate("", rules...)
}

func (c *MuxPoolConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Field("generalCount", c.GeneralCount, validation.GTE(1)),
		validation.Field("replicationCount", c.ReplicationCount, validation.GTE(1)),
	)
}

func (c MuxPoolConfig) Total() int {
	return c.GeneralCount + c.ReplicationCount
}
