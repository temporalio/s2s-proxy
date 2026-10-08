package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentialsConfigValidate(t *testing.T) {
	identity := func(identity CredentialIdentity) *CredentialsConfig {
		return &CredentialsConfig{Identity: identity}
	}
	tests := []struct {
		name      string
		conn      ClusterConnConfig
		wantError string
	}{
		{
			name: "no credentials block",
			conn: ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeMuxClient}},
		},
		{
			name: "local tcp with proxy identity",
			conn: ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeTCP, Credentials: identity(CredentialIdentityProxy)}},
		},
		{
			name: "local mux with default identity",
			conn: ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeMuxClient, Credentials: identity(CredentialIdentityDefault)}},
		},
		{
			name: "local mux with strip identity",
			conn: ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeMuxClient, Credentials: identity(CredentialIdentityStrip)}},
		},
		{
			name:      "local mux with proxy identity",
			conn:      ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeMuxClient, Credentials: identity(CredentialIdentityProxy)}},
			wantError: `identity "proxy" requires connectionType "tcp", got "mux-client"`,
		},
		{
			name:      "unsupported identity",
			conn:      ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeTCP, Credentials: identity("everyone")}},
			wantError: `unsupported identity "everyone"`,
		},
		{
			name:      "remote with credentials",
			conn:      ClusterConnConfig{Remote: ClusterDefinition{ConnectionType: ConnTypeTCP, Credentials: identity(CredentialIdentityProxy)}},
			wantError: "credentials are only supported on the local cluster definition",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.conn.Validate()
			if tc.wantError == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.wantError)
		})
	}
}

func TestCredentialIdentityDefault(t *testing.T) {
	require.Equal(t, CredentialIdentityDefault, ClusterDefinition{}.CredentialIdentity())
	require.Equal(t, CredentialIdentityDefault, ClusterDefinition{Credentials: &CredentialsConfig{}}.CredentialIdentity())
	require.Equal(t, CredentialIdentityProxy,
		ClusterDefinition{Credentials: &CredentialsConfig{Identity: CredentialIdentityProxy}}.CredentialIdentity())
}
