package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/encryption"
)

func TestCallCredentialsConfigValidation(t *testing.T) {
	tests := []struct {
		name       string
		definition ClusterDefinition
		wantError  string
	}{
		{
			name: "valid",
			definition: ClusterDefinition{
				ConnectionType: ConnTypeTCP,
				TcpClient: TCPTLSInfo{TLSConfig: encryption.TLSConfig{
					CAServerName: "frontend.example.invalid",
				}},
				CallCredentials: &CallCredentialsConfig{Provider: "provider"},
			},
		},
		{
			name: "valid required mode",
			definition: ClusterDefinition{
				ConnectionType: ConnTypeTCP,
				TcpClient: TCPTLSInfo{TLSConfig: encryption.TLSConfig{
					CAServerName: "frontend.example.invalid",
				}},
				CallCredentials: &CallCredentialsConfig{Provider: "provider", Mode: "required"},
			},
		},
		{
			name: "missing provider",
			definition: ClusterDefinition{
				ConnectionType: ConnTypeTCP,
				TcpClient: TCPTLSInfo{TLSConfig: encryption.TLSConfig{
					CAServerName: "frontend.example.invalid",
				}},
				CallCredentials: &CallCredentialsConfig{},
			},
			wantError: "provider",
		},
		{
			name: "unsupported mode",
			definition: ClusterDefinition{
				ConnectionType: ConnTypeTCP,
				TcpClient: TCPTLSInfo{TLSConfig: encryption.TLSConfig{
					CAServerName: "frontend.example.invalid",
				}},
				CallCredentials: &CallCredentialsConfig{Provider: "provider", Mode: "optional"},
			},
			wantError: "unsupported mode",
		},
		{
			name: "plaintext",
			definition: ClusterDefinition{
				ConnectionType:  ConnTypeTCP,
				CallCredentials: &CallCredentialsConfig{Provider: "provider"},
			},
			wantError: "require TLS",
		},
		{
			name: "missing server name",
			definition: ClusterDefinition{
				ConnectionType: ConnTypeTCP,
				TcpClient: TCPTLSInfo{TLSConfig: encryption.TLSConfig{
					CertificatePath: "client.pem",
					KeyPath:         "client.key",
				}},
				CallCredentials: &CallCredentialsConfig{Provider: "provider"},
			},
			wantError: "require a CA server name",
		},
		{
			name: "CA verification disabled",
			definition: ClusterDefinition{
				ConnectionType: ConnTypeTCP,
				TcpClient: TCPTLSInfo{TLSConfig: encryption.TLSConfig{
					CAServerName:       "frontend.example.invalid",
					SkipCAVerification: true,
				}},
				CallCredentials: &CallCredentialsConfig{Provider: "provider"},
			},
			wantError: "require CA verification",
		},
		{
			name: "mux client",
			definition: ClusterDefinition{
				ConnectionType:  ConnTypeMuxClient,
				CallCredentials: &CallCredentialsConfig{Provider: "provider"},
			},
			wantError: "require a TCP destination",
		},
		{
			name: "mux server",
			definition: ClusterDefinition{
				ConnectionType:  ConnTypeMuxServer,
				CallCredentials: &CallCredentialsConfig{Provider: "provider"},
			},
			wantError: "require a TCP destination",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.definition.Validate()
			if test.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestCallCredentialsYAML(t *testing.T) {
	config, err := LoadConfig[S2SProxyConfig](writeTestConfig(t, `
clusterConnections:
  - name: migration
    local:
      connectionType: tcp
      tcpClient:
        address: frontend.example.invalid:7233
        tls:
          caServerName: frontend.example.invalid
          skipCAVerification: false
      callCredentials:
        provider: custom
        properties:
          audience: frontend
    remote:
      connectionType: mux-server
`))
	require.NoError(t, err)
	require.Len(t, config.ClusterConnections, 1)
	credentials := config.ClusterConnections[0].Local.CallCredentials
	require.NotNil(t, credentials)
	assert.Equal(t, "custom", credentials.Provider)
	assert.Equal(t, "frontend", credentials.Properties["audience"])
	require.NoError(t, config.Validate())
}

func TestConfigurationWithoutCallCredentialsRemainsValid(t *testing.T) {
	definition := ClusterDefinition{ConnectionType: ConnTypeMuxServer}
	require.NoError(t, definition.Validate())
}

func writeTestConfig(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o600))
	return path
}
