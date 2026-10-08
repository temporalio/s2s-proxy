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
		ConnectionType ConnectionType `yaml:"connectionType"`
		TcpClient      TCPTLSInfo     `yaml:"tcpClient"`
		TcpServer      TCPTLSInfo     `yaml:"tcpServer"`
		MuxCount       int            `yaml:"muxCount"`
		MuxAddressInfo TCPTLSInfo     `yaml:"muxAddressInfo"`
		// Credentials controls which identity calls on this connection present to the cluster.
		// Only the local cluster definition supports it.
		Credentials *CredentialsConfig `yaml:"credentials"`
	}

	CredentialsConfig struct {
		// Identity is whose credentials the cluster sees. Empty means CredentialIdentityCaller.
		Identity CredentialIdentity `yaml:"identity"`
	}

	// CredentialIdentity is whose credentials a cluster sees on calls from the proxy. The credential headers are
	// "authorization" and "authorization-extras".
	CredentialIdentity string

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
	// CredentialIdentityCaller forwards whatever credentials the caller sent, and adds none. This is the default.
	CredentialIdentityCaller CredentialIdentity = "caller"
	// CredentialIdentityProxy drops forwarded credentials and sends the auth.CredentialProvider's instead.
	CredentialIdentityProxy CredentialIdentity = "proxy"
	// CredentialIdentityNone drops forwarded credentials and sends none.
	CredentialIdentityNone CredentialIdentity = "none"
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

// CredentialIdentity returns whose credentials this cluster sees, defaulting to CredentialIdentityCaller.
func (c ClusterDefinition) CredentialIdentity() CredentialIdentity {
	if c.Credentials == nil || c.Credentials.Identity == "" {
		return CredentialIdentityCaller
	}
	return c.Credentials.Identity
}

// Validate reports problems in this connection's config. Only the encryption
// and credentials blocks are covered so far; the checks NewProxy makes inline could move here.
func (c *ClusterConnConfig) Validate() error {
	return validation.Validate(
		"",
		validation.Nested("encryption", &c.EncryptionConfig),
		validation.Field("local.credentials.identity", c.Local, func(local ClusterDefinition) error {
			switch identity := local.CredentialIdentity(); identity {
			case CredentialIdentityCaller, CredentialIdentityNone:
				return nil
			case CredentialIdentityProxy:
				if local.ConnectionType != ConnTypeTCP {
					return fmt.Errorf("identity %q requires connectionType %q, got %q",
						identity, ConnTypeTCP, local.ConnectionType)
				}
				return nil
			default:
				return fmt.Errorf("unsupported identity %q: must be %q, %q or %q",
					identity, CredentialIdentityCaller, CredentialIdentityProxy, CredentialIdentityNone)
			}
		}),
		validation.Field("remote.credentials", c.Remote.Credentials, func(credentials *CredentialsConfig) error {
			if credentials != nil {
				return errors.New("credentials are only supported on the local cluster definition")
			}
			return nil
		}),
	)
}
