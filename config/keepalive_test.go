package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/keepalive"
)

func TestKeepaliveFromYAML(t *testing.T) {
	path := writeYAML(t, `
clusterConnections:
  - name: cluster-a
    local:
      connectionType: tcp
      keepalive:
        client:
          time: 30s
          timeout: 20s
          permitWithoutStream: true
        server:
          time: 1m
          timeout: 20s
          enforcementPolicy:
            minTime: 10s
            permitWithoutStream: true
    remote:
      connectionType: mux-client
`)
	cfg, err := LoadConfig[S2SProxyConfig](path)
	require.NoError(t, err)
	require.NoError(t, cfg.Validate())

	local := cfg.ClusterConnections[0].Local
	require.Equal(t, keepalive.ClientParameters{
		Time:                30 * time.Second,
		Timeout:             20 * time.Second,
		PermitWithoutStream: true,
	}, local.ClientKeepalive().Parameters())
	require.Equal(t, keepalive.ServerParameters{
		Time:    time.Minute,
		Timeout: 20 * time.Second,
	}, local.ServerKeepalive().Parameters())
	require.Equal(t, keepalive.EnforcementPolicy{
		MinTime:             10 * time.Second,
		PermitWithoutStream: true,
	}, local.ServerKeepalive().EnforcementPolicy.Policy())

	remote := cfg.ClusterConnections[0].Remote
	require.Nil(t, remote.Keepalive)
	require.Nil(t, remote.ClientKeepalive())
	require.Nil(t, remote.ServerKeepalive())
}

func TestKeepaliveValidate(t *testing.T) {
	cases := []struct {
		name    string
		cfg     *KeepaliveConfig
		wantErr []string
	}{
		{name: "nil block"},
		{name: "empty block", cfg: &KeepaliveConfig{}},
		{
			name: "client time zero means no pings",
			cfg:  &KeepaliveConfig{Client: &ClientKeepaliveConfig{Timeout: 20 * time.Second}},
		},
		{
			name: "client time at the grpc-go minimum",
			cfg:  &KeepaliveConfig{Client: &ClientKeepaliveConfig{Time: MinClientKeepaliveTime}},
		},
		{
			name:    "client time below the grpc-go minimum",
			cfg:     &KeepaliveConfig{Client: &ClientKeepaliveConfig{Time: 5 * time.Second}},
			wantErr: []string{"client.time", "at least 10s"},
		},
		{
			name: "negative durations",
			cfg: &KeepaliveConfig{
				Client: &ClientKeepaliveConfig{Timeout: -time.Second},
				Server: &ServerKeepaliveConfig{
					Time:              -time.Second,
					Timeout:           -time.Second,
					EnforcementPolicy: &KeepaliveEnforcementPolicyConfig{MinTime: -time.Second},
				},
			},
			wantErr: []string{"client.timeout", "server.time", "server.timeout", "server.enforcementPolicy.minTime"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.cfg.Validate()
			if c.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range c.wantErr {
				require.ErrorContains(t, err, want)
			}
		})
	}
}

func TestClusterConnConfigValidatesKeepalive(t *testing.T) {
	cfg := S2SProxyConfig{ClusterConnections: []ClusterConnConfig{{
		Name: "cluster-a",
		Local: ClusterDefinition{
			Keepalive: &KeepaliveConfig{Client: &ClientKeepaliveConfig{Time: time.Second}},
		},
		Remote: ClusterDefinition{
			Keepalive: &KeepaliveConfig{Server: &ServerKeepaliveConfig{Time: -time.Second}},
		},
	}}}
	err := cfg.Validate()
	require.ErrorContains(t, err, "clusterConnections[0].local.keepalive")
	require.ErrorContains(t, err, "clusterConnections[0].remote.keepalive")
}
