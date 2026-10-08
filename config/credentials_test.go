package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCredentialsConfigValidate(t *testing.T) {
	enabled := &CredentialsConfig{Enabled: true}
	tests := []struct {
		name      string
		conn      ClusterConnConfig
		wantError string
	}{
		{
			name: "local tcp with credentials",
			conn: ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeTCP, Credentials: enabled}},
		},
		{
			name: "local mux with credentials disabled",
			conn: ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeMuxClient, Credentials: &CredentialsConfig{}}},
		},
		{
			name:      "local mux with credentials enabled",
			conn:      ClusterConnConfig{Local: ClusterDefinition{ConnectionType: ConnTypeMuxClient, Credentials: enabled}},
			wantError: `credentials require connectionType "tcp", got "mux-client"`,
		},
		{
			name:      "remote with credentials",
			conn:      ClusterConnConfig{Remote: ClusterDefinition{ConnectionType: ConnTypeTCP, Credentials: enabled}},
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

func TestCredentialsEnabled(t *testing.T) {
	require.False(t, ClusterDefinition{}.CredentialsEnabled())
	require.False(t, ClusterDefinition{Credentials: &CredentialsConfig{}}.CredentialsEnabled())
	require.True(t, ClusterDefinition{Credentials: &CredentialsConfig{Enabled: true}}.CredentialsEnabled())
}
