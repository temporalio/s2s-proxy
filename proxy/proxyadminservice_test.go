package proxy

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/adminplane"
	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

const (
	stateConnected   = proxyadminv1.ConnectionState_CONNECTION_STATE_CONNECTED
	stateError       = proxyadminv1.ConnectionState_CONNECTION_STATE_ERROR
	stateUnspecified = proxyadminv1.ConnectionState_CONNECTION_STATE_UNSPECIFIED
)

type memberConn struct {
	name                     string
	state                    proxyadminv1.ConnectionState
	connected, total, target int32
}

// memberResponse builds one member's member-scoped answer.
func memberResponse(id, version string, conns ...memberConn) *proxyadminv1.DescribeClusterConnectionsResponse {
	resp := &proxyadminv1.DescribeClusterConnectionsResponse{}
	for _, c := range conns {
		member := &proxyadminv1.ClusterConnectionMember{
			// Every pod marks its own row. The merge decides which one survives as self.
			Identity:             &proxyadminv1.Member{Id: id, Self: true, Version: version},
			State:                c.state,
			MuxSessionsConnected: c.connected,
			MuxSessionsTotal:     c.total,
			MuxSessionsTarget:    c.target,
		}
		resp.ClusterConnections = append(resp.ClusterConnections, &proxyadminv1.ClusterConnection{
			Name:                 c.name,
			State:                c.state,
			MuxSessionsConnected: c.connected,
			MuxSessionsTotal:     c.total,
			MuxSessionsTarget:    c.target,
			Members:              []*proxyadminv1.ClusterConnectionMember{member},
		})
	}
	return resp
}

func result(resp *proxyadminv1.DescribeClusterConnectionsResponse) adminplane.Result[describeResponse] {
	return adminplane.Result[describeResponse]{ID: selfIdentity(resp).GetId(), Value: resp}
}

func memberByID(t *testing.T, cc *proxyadminv1.ClusterConnection, id string) *proxyadminv1.ClusterConnectionMember {
	t.Helper()
	for _, m := range cc.GetMembers() {
		if m.GetIdentity().GetId() == id {
			return m
		}
	}
	t.Fatalf("no member %q in %v", id, cc)
	return nil
}

func connectionByName(t *testing.T, resp *proxyadminv1.DescribeClusterConnectionsResponse, name string) *proxyadminv1.ClusterConnection {
	t.Helper()
	for _, cc := range resp.GetClusterConnections() {
		if cc.GetName() == name {
			return cc
		}
	}
	t.Fatalf("no cluster connection named %q in %v", name, resp)
	return nil
}

func TestMergeSumsSessionsAcrossMembers(t *testing.T) {
	self := result(memberResponse("pod-a", "v1", memberConn{name: "cluster-b", state: stateConnected, connected: 7, total: 7, target: 7}))
	peer := result(memberResponse("pod-b", "v1", memberConn{name: "cluster-b", state: stateConnected, connected: 6, total: 6, target: 6}))

	merged := mergeDescribeClusterConnections(self, []adminplane.Result[describeResponse]{peer},
		adminplane.Roster{Provider: "static", Discovered: 2, Responding: 2})

	cc := connectionByName(t, merged, "cluster-b")
	require.Equal(t, stateConnected, cc.GetState())
	require.Equal(t, int32(13), cc.GetMuxSessionsConnected())
	require.Equal(t, int32(13), cc.GetMuxSessionsTotal())
	require.Equal(t, int32(13), cc.GetMuxSessionsTarget())
	require.Len(t, cc.GetMembers(), 2)
	require.Equal(t, "v1", memberByID(t, cc, "pod-a").GetIdentity().GetVersion())
}

// A member that is missing a connection another member has is a half-applied configuration, which
// is precisely what an aggregated view exists to reveal. Intersecting the names would hide it.
func TestMergeReportsConnectionMissingFromOneMember(t *testing.T) {
	self := result(memberResponse("pod-a", "v1",
		memberConn{name: "cluster-b", state: stateConnected, connected: 4, total: 4, target: 4},
		memberConn{name: "cluster-c", state: stateConnected, connected: 4, total: 4, target: 4}))
	peer := result(memberResponse("pod-b", "v1",
		memberConn{name: "cluster-b", state: stateConnected, connected: 4, total: 4, target: 4}))

	merged := mergeDescribeClusterConnections(self, []adminplane.Result[describeResponse]{peer},
		adminplane.Roster{Provider: "static", Discovered: 2, Responding: 2})

	clusterC := connectionByName(t, merged, "cluster-c")
	require.Equal(t, stateError, clusterC.GetState())
	require.Len(t, clusterC.GetMembers(), 2)

	absent := memberByID(t, clusterC, "pod-b")
	require.Equal(t, stateError, absent.GetState())
	// A target of zero separates a member that lacks the connection from one that has it and cannot hold a session.
	require.Zero(t, absent.GetMuxSessionsTarget())
	require.Equal(t, int32(4), clusterC.GetMuxSessionsConnected(), "the absent member contributes no sessions")
	require.Equal(t, int32(4), clusterC.GetMuxSessionsTarget())
}

// A member that never answered is absent from members[].
// Without an explicit rule the remaining members would make the deployment look entirely healthy.
func TestMergeUnreachableMemberErrorsEveryConnection(t *testing.T) {
	self := result(memberResponse("pod-a", "v1",
		memberConn{name: "cluster-b", state: stateConnected, connected: 7, total: 7, target: 7},
		memberConn{name: "cluster-c", state: stateConnected, connected: 7, total: 7, target: 7}))

	merged := mergeDescribeClusterConnections(self, nil, adminplane.Roster{
		Provider: "dns", Discovered: 2, Responding: 1,
		Unreachable: []adminplane.Unreachable{{Address: "10.0.0.9:9234", Message: "connection refused", Code: 14}},
	})

	// The member that failed to answer has no row: it reported nothing to put in one.
	// Erroring every connection is the only trace it leaves.
	require.Len(t, merged.GetClusterConnections(), 2)
	for _, cc := range merged.GetClusterConnections() {
		require.Equal(t, stateError, cc.GetState())
		require.Len(t, cc.GetMembers(), 1)
	}
}

func TestMergeReportsVersionSkewPerMember(t *testing.T) {
	self := result(memberResponse("pod-a", "v1.4.2",
		memberConn{name: "cluster-b", state: stateConnected, connected: 7, total: 7, target: 7}))
	peer := result(memberResponse("pod-b", "v1.4.1",
		memberConn{name: "cluster-b", state: stateError, total: 7, target: 7}))

	merged := mergeDescribeClusterConnections(self, []adminplane.Result[describeResponse]{peer},
		adminplane.Roster{Provider: "dns", Discovered: 2, Responding: 2})

	cc := connectionByName(t, merged, "cluster-b")
	require.Equal(t, "v1.4.2", memberByID(t, cc, "pod-a").GetIdentity().GetVersion())
	require.Equal(t, "v1.4.1", memberByID(t, cc, "pod-b").GetIdentity().GetVersion())
	require.Equal(t, stateError, cc.GetState(), "one member connected and one errored is not connected")
}

// Only the aggregator knows which pod served the response.
// Every reporting pod's own claim to self has to be cleared except the one that answered.
func TestMergeMarksOnlyTheRespondingPodAsSelf(t *testing.T) {
	self := result(memberResponse("pod-a", "v1", memberConn{name: "cluster-b", state: stateConnected, connected: 1, total: 1, target: 1}))
	peer := result(memberResponse("pod-b", "v1", memberConn{name: "cluster-b", state: stateConnected, connected: 1, total: 1, target: 1}))

	merged := mergeDescribeClusterConnections(self, []adminplane.Result[describeResponse]{peer},
		adminplane.Roster{Provider: "static", Discovered: 2, Responding: 2})

	cc := connectionByName(t, merged, "cluster-b")
	require.True(t, memberByID(t, cc, "pod-a").GetIdentity().GetSelf())
	require.False(t, memberByID(t, cc, "pod-b").GetIdentity().GetSelf())
}

// Not-multiplexed connections carry no session state.
// They must not drag the rollup to ERROR.
func TestMergeAllUnspecifiedStaysUnspecified(t *testing.T) {
	self := result(memberResponse("pod-a", "v1", memberConn{name: "tcp-conn", state: stateUnspecified}))
	peer := result(memberResponse("pod-b", "v1", memberConn{name: "tcp-conn", state: stateUnspecified}))

	merged := mergeDescribeClusterConnections(self, []adminplane.Result[describeResponse]{peer},
		adminplane.Roster{Provider: "static", Discovered: 2, Responding: 2})

	require.Equal(t, stateUnspecified, connectionByName(t, merged, "tcp-conn").GetState())
}

func TestMergeSortsMembersById(t *testing.T) {
	self := result(memberResponse("pod-z", "v1", memberConn{name: "cluster-b", state: stateConnected, connected: 1, total: 1, target: 1}))
	peers := []adminplane.Result[describeResponse]{
		result(memberResponse("pod-m", "v1", memberConn{name: "cluster-b", state: stateConnected, connected: 1, total: 1, target: 1})),
		result(memberResponse("pod-a", "v1", memberConn{name: "cluster-b", state: stateConnected, connected: 1, total: 1, target: 1})),
	}

	merged := mergeDescribeClusterConnections(self, peers,
		adminplane.Roster{Provider: "static", Discovered: 3, Responding: 3})

	var ids []string
	for _, m := range connectionByName(t, merged, "cluster-b").GetMembers() {
		ids = append(ids, m.GetIdentity().GetId())
	}
	require.Equal(t, []string{"pod-a", "pod-m", "pod-z"}, ids)
}

func TestViewNarrowsToTheArrivingConnectionForACounterparty(t *testing.T) {
	resp := memberResponse("pod-a", "v1",
		memberConn{name: "cluster-b", state: stateConnected, connected: 1, total: 1, target: 1},
		memberConn{name: "cluster-c", state: stateConnected, connected: 1, total: 1, target: 1})
	narrowed := viewDescribeClusterConnections(resp,
		adminplane.ServerOptions{Role: adminplane.RoleCounterparty, ConnectionName: "cluster-b"})

	require.Len(t, narrowed.GetClusterConnections(), 1)
	cc := narrowed.GetClusterConnections()[0]
	require.Equal(t, "cluster-b", cc.GetName())
	require.Equal(t, stateConnected, cc.GetState())
	require.Equal(t, int32(1), cc.GetMuxSessionsConnected())
	// Sessions connected reads as healthy on its own. The target is what makes it alertable.
	require.Equal(t, int32(1), cc.GetMuxSessionsTarget())

	// This deployment's shape does not cross the boundary.
	// Pod ids, addresses and versions say where this deployment's pods are.
	// How many rows there are says how many it runs.
	require.Empty(t, cc.GetMembers(), "per-pod detail names our pods")
}

// The response is built field by field.
// A name that matches nothing yields an empty answer rather than the whole thing.
func TestViewYieldsNothingWhenTheConnectionNameMatchesNothing(t *testing.T) {
	resp := memberResponse("pod-a", "v1",
		memberConn{name: "cluster-b", state: stateConnected, connected: 1, total: 1, target: 1})

	narrowed := viewDescribeClusterConnections(resp,
		adminplane.ServerOptions{Role: adminplane.RoleCounterparty, ConnectionName: "cluster-z"})

	require.Empty(t, narrowed.GetClusterConnections())
}

func TestViewLeavesOperatorResponsesIntact(t *testing.T) {
	resp := memberResponse("pod-a", "v1",
		memberConn{name: "cluster-b", state: stateConnected, connected: 1, total: 1, target: 1},
		memberConn{name: "cluster-c", state: stateConnected, connected: 1, total: 1, target: 1})

	kept := viewDescribeClusterConnections(resp, adminplane.ServerOptions{Role: adminplane.RoleOperator})

	require.Len(t, kept.GetClusterConnections(), 2)
	require.Len(t, kept.GetClusterConnections()[0].GetMembers(), 1)
}

func TestSessionState(t *testing.T) {
	cases := []struct {
		name   string
		counts mux.SessionCounts
		target int
		want   proxyadminv1.ConnectionState
	}{
		{name: "every configured session connected", counts: mux.SessionCounts{Connected: 10, Total: 10}, target: 10, want: stateConnected},
		// A session that never established was never added to the manager.
		// The sessions held read as fully connected on their own.
		{name: "fewer sessions held than configured", counts: mux.SessionCounts{Connected: 3, Total: 3}, target: 10, want: stateError},
		{name: "partially connected", counts: mux.SessionCounts{Connected: 1, Total: 10}, target: 10, want: stateError},
		{name: "no sessions", counts: mux.SessionCounts{}, target: 10, want: stateError},
		{name: "errored only", counts: mux.SessionCounts{Errored: 3, Total: 3}, target: 3, want: stateError},
		{name: "closed only", counts: mux.SessionCounts{Closed: 3, Total: 3}, target: 3, want: stateError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, sessionState(c.counts, c.target))
		})
	}
}

// rollup is the group-scope answer.
// Group is the default scope.
// A state it cannot represent is the state most callers will read.
func TestRollup(t *testing.T) {
	cases := []struct {
		name           string
		states         []proxyadminv1.ConnectionState
		anyUnreachable bool
		want           proxyadminv1.ConnectionState
	}{
		{name: "all connected", states: []proxyadminv1.ConnectionState{stateConnected, stateConnected}, want: stateConnected},
		{name: "one member errored", states: []proxyadminv1.ConnectionState{stateConnected, stateError}, want: stateError},
		{name: "all errored", states: []proxyadminv1.ConnectionState{stateError, stateError}, want: stateError},
		// Not multiplexed: no session state to contribute.
		{name: "all unspecified", states: []proxyadminv1.ConnectionState{stateUnspecified}, want: stateUnspecified},
		// One connection of the two is multiplexed.
		// It is the only one with a state to report.
		{name: "unspecified alongside connected", states: []proxyadminv1.ConnectionState{stateUnspecified, stateConnected}, want: stateConnected},
		{name: "unspecified alongside errored", states: []proxyadminv1.ConnectionState{stateUnspecified, stateError}, want: stateError},
		// A member that never answered is not evidence of health.
		{
			name:   "connected but a member did not answer",
			states: []proxyadminv1.ConnectionState{stateConnected}, anyUnreachable: true, want: stateError,
		},
		// Nothing multiplexed reported anything.
		// An unreachable member has no state to degrade.
		{
			name:   "unspecified and a member did not answer",
			states: []proxyadminv1.ConnectionState{stateUnspecified}, anyUnreachable: true, want: stateUnspecified,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, rollup(c.states, c.anyUnreachable))
		})
	}
}
