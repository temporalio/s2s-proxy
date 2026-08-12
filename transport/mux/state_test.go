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

// stubManager reports a fixed session set. CountSessions reaches only GetMuxConnections.
type stubManager struct {
	MultiMuxManager
	sessions map[string]session.ManagedMuxSession
}

func (s stubManager) GetMuxConnections() map[string]session.ManagedMuxSession { return s.sessions }

type stubSession struct {
	session.ManagedMuxSession
	info *session.MuxSessionInfo
}

func (s stubSession) State() *session.MuxSessionInfo { return s.info }

func TestCountSessions(t *testing.T) {
	inState := func(st session.MuxSessionState) session.ManagedMuxSession {
		return stubSession{info: &session.MuxSessionInfo{State: st}}
	}

	cases := []struct {
		name    string
		manager MultiMuxManager
		want    SessionCounts
	}{
		{
			// Reached when a cluster connection is not multiplexed.
			name:    "a nil manager holds nothing",
			manager: nil,
			want:    SessionCounts{},
		},
		{
			name:    "no sessions",
			manager: stubManager{sessions: map[string]session.ManagedMuxSession{}},
			want:    SessionCounts{},
		},
		{
			name: "one of each",
			manager: stubManager{sessions: map[string]session.ManagedMuxSession{
				"a": inState(session.Connected),
				"b": inState(session.Closed),
				"c": inState(session.Error),
			}},
			want: SessionCounts{Connected: 1, Closed: 1, Errored: 1, Total: 3},
		},
		{
			// A session with no recorded state counts toward Total and nothing else.
			name: "state not yet recorded",
			manager: stubManager{sessions: map[string]session.ManagedMuxSession{
				"a": stubSession{info: nil},
			}},
			want: SessionCounts{Total: 1},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, CountSessions(c.manager))
		})
	}
}

func TestSessionCountsString(t *testing.T) {
	require.Equal(t, "connected=0/0 errored=0 closed=0", SessionCounts{}.String())
	require.Equal(t, "connected=3/5 errored=1 closed=1",
		SessionCounts{Connected: 3, Errored: 1, Closed: 1, Total: 5}.String())
}
