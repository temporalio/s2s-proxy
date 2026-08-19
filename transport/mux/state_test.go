package mux

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport/mux/session"
)

func TestSessionStateName(t *testing.T) {
	cases := []struct {
		name string
		info *session.MuxSessionInfo
		want string
	}{
		{name: "connected", info: &session.MuxSessionInfo{State: session.Connected}, want: SessionStateConnected},
		{name: "closed", info: &session.MuxSessionInfo{State: session.Closed}, want: SessionStateClosed},
		{name: "errored", info: &session.MuxSessionInfo{State: session.Error}, want: SessionStateErrored},
		// A session added to the manager before its first health check has no recorded state.
		{name: "not recorded", info: nil, want: SessionStateUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, SessionStateName(c.info))
		})
	}
}

func TestDesiredMuxCount(t *testing.T) {
	require.Equal(t, 7, DesiredMuxCount(config.ClusterDefinition{MuxCount: 7}))
	require.Equal(t, 10, DesiredMuxCount(config.ClusterDefinition{}))
}
