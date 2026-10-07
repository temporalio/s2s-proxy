package config

import (
	"errors"
	"fmt"

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
		ConnectionType  ConnectionType         `yaml:"connectionType"`
		TcpClient       TCPTLSInfo             `yaml:"tcpClient"`
		TcpServer       TCPTLSInfo             `yaml:"tcpServer"`
		MuxCount        int                    `yaml:"muxCount"`
		MuxAddressInfo  TCPTLSInfo             `yaml:"muxAddressInfo"`
		CallCredentials *CallCredentialsConfig `yaml:"callCredentials"`
	}

	CallCredentialsConfig struct {
		Provider   string            `yaml:"provider"`
		Mode       string            `yaml:"mode"`
		Properties map[string]string `yaml:"properties"`
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

// Validate reports problems in this connection's config. Only the encryption
// block is covered so far; the checks NewProxy makes inline could move here.
func (c *ClusterConnConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Nested("local", &c.Local),
		validation.Nested("remote", &c.Remote),
		validation.Nested("encryption", &c.EncryptionConfig),
	)
}

func (c *ClusterDefinition) Validate() error {
	if c.CallCredentials == nil {
		return nil
	}

	return validation.Validate(
		"",
		validation.Nested("callCredentials", c.CallCredentials),
		validation.Field("connectionType", c.ConnectionType, func(connectionType ConnectionType) error {
			if connectionType != ConnTypeTCP {
				return errors.New("call credentials require a TCP destination")
			}
			return nil
		}),
		validation.Field("tcpClient.tls", c.TcpClient.TLSConfig, func(tlsConfig encryption.TLSConfig) error {
			if !tlsConfig.IsEnabled() {
				return errors.New("call credentials require TLS")
			}
			if tlsConfig.SkipCAVerification {
				return errors.New("call credentials require CA verification")
			}
			if tlsConfig.CAServerName == "" {
				return errors.New("call credentials require a CA server name")
			}
			return nil
		}),
	)
}

func (c *CallCredentialsConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Field("provider", c.Provider, validation.Required[string]()),
		validation.Field("mode", c.Mode, func(mode string) error {
			if mode != "" && mode != "required" {
				return fmt.Errorf("unsupported mode %q", mode)
			}
			return nil
		}),
	)
}
