package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/adminplane"
	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
)

// dynamicAddress reserves a localhost address and releases it.
// A listener can bind it next.
func dynamicAddress(t *testing.T) string {
	t.Helper()
	return getDynamicPorts(t, 1)[0]
}

// buildPeerProxyAt starts a proxy whose peer listener binds peerAddr and whose static discovery list is peers.
// A static list keeps the group deterministic, unlike DNS in a unit test.
func buildPeerProxyAt(t *testing.T, memberID, connName, peerAddr string, peers []string) (*Proxy, string) {
	t.Helper()
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	a := getDynamicPlccAddresses(t)
	operatorAddr := dynamicAddress(t)

	cfg := config.S2SProxyConfig{
		ProxyAdmin: config.ProxyAdminConfig{
			ListenAddress: operatorAddr,
			Peer: &config.ProxyAdminPeerConfig{
				ListenAddress: peerAddr,
				Discovery: config.DiscoveryConfig{
					Provider: config.DiscoveryStatic,
					Static:   config.StaticDiscoveryConfig{Addresses: peers},
				},
			},
		},
		ClusterConnections: []config.ClusterConnConfig{
			makeMuxClusterConfig(connName, config.ConnTypeMuxServer, localFVI, remoteFVI,
				a.localProxyOutbound, a.localTemporalAddr, a.localProxyOutbound, a.localProxyInbound),
		},
	}

	proxy, err := NewProxy(config.NewMockConfigProvider(cfg), loggers,
		Identity{Version: "v-test", MemberID: memberID})
	require.NoError(t, err)
	require.NoError(t, proxy.Start())
	t.Cleanup(proxy.Stop)
	return proxy, operatorAddr
}

func adminClient(t *testing.T, address string) proxyadminv1.ProxyAdminServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return proxyadminv1.NewProxyAdminServiceClient(conn)
}

// Two proxies discover each other and each answers for the pair.
// A single call describes the deployment, not the pod that happened to receive it.
func TestGroupScopeAggregatesBothMembers(t *testing.T) {
	// Allocate the peer addresses first so each proxy can list both.
	peerA := dynamicAddress(t)
	peerB := dynamicAddress(t)
	peers := []string{peerA, peerB}

	proxyA, operatorA := buildPeerProxyAt(t, "pod-a", "conn-a", peerA, peers)
	proxyB, _ := buildPeerProxyAt(t, "pod-b", "conn-b", peerB, peers)

	client := adminClient(t, operatorA)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var resp *proxyadminv1.DescribeClusterConnectionsResponse
	require.Eventually(t, func() bool {
		var err error
		resp, err = client.DescribeClusterConnections(ctx, &proxyadminv1.DescribeClusterConnectionsRequest{})
		return err == nil && respondingMembers(resp) == 2
	}, 20*time.Second, 200*time.Millisecond, "the two members never both answered")

	// The answering member names itself.
	// The other member is not lost.
	// Both proxies dial every address, their own included.
	// This also proves self is deduplicated by id rather than counted twice.
	require.Equal(t, proxyA.MemberID(), selfMemberID(resp))
	require.NotEqual(t, proxyA.MemberID(), proxyB.MemberID())
	require.Equal(t, 1, selfRows(resp), "exactly one row per connection is self")

	// The union of both members' connections, each reported by both members.
	names := map[string]int{}
	for _, cc := range resp.GetClusterConnections() {
		names[cc.GetName()] = len(cc.GetMembers())
	}
	require.Equal(t, map[string]int{"conn-a": 2, "conn-b": 2}, names,
		"each connection should list every responding member, including the one that lacks it")
}

// A member scoped call answers for one process.
// It never fans out and carries no roster.
func TestMemberScopeDoesNotFanOut(t *testing.T) {
	peerA := dynamicAddress(t)
	_, operatorA := buildPeerProxyAt(t, "pod-a", "conn-a", peerA, []string{peerA})

	client := adminClient(t, operatorA)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	resp, err := client.DescribeClusterConnections(
		metadata.AppendToOutgoingContext(ctx, adminplane.MDScope, "member"),
		&proxyadminv1.DescribeClusterConnectionsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetClusterConnections(), 1)
	require.Len(t, resp.GetClusterConnections()[0].GetMembers(), 1)
}

// A discovered member that is not listening leaves no row of its own.
// It reported nothing.
// There is nothing to report it in.
//
// It is not necessarily visible at all.
// The unreachable member only holds a connection that would otherwise read CONNECTED down to ERROR.
// TestRollup covers that case.
// This connection is a mux-server with nothing dialed in.
// It reads ERROR either way.
// The dead member leaves no trace.
func TestUnreachableMemberLeavesNoRow(t *testing.T) {
	peerA := dynamicAddress(t)
	// Reserved and released.
	// Nothing is listening there.
	dead := dynamicAddress(t)

	_, operatorA := buildPeerProxyAt(t, "pod-a", "conn-a", peerA, []string{peerA, dead})

	client := adminClient(t, operatorA)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var resp *proxyadminv1.DescribeClusterConnectionsResponse
	require.Eventually(t, func() bool {
		var err error
		resp, err = client.DescribeClusterConnections(ctx, &proxyadminv1.DescribeClusterConnectionsRequest{})
		return err == nil && len(resp.GetClusterConnections()) == 1
	}, 20*time.Second, 200*time.Millisecond, "the live member never answered")

	// One row, from the member that answered.
	// Nothing anywhere names the dead one.
	require.Len(t, resp.GetClusterConnections()[0].GetMembers(), 1)
	require.Equal(t, "pod-a", resp.GetClusterConnections()[0].GetMembers()[0].GetIdentity().GetId())
}

// respondingMembers counts the rows in the first connection.
// That is one row per member that answered.
func respondingMembers(resp *proxyadminv1.DescribeClusterConnectionsResponse) int {
	if len(resp.GetClusterConnections()) == 0 {
		return 0
	}
	return len(resp.GetClusterConnections()[0].GetMembers())
}

// selfMemberID returns the id of the pod that served the response.
func selfMemberID(resp *proxyadminv1.DescribeClusterConnectionsResponse) string {
	for _, cc := range resp.GetClusterConnections() {
		for _, m := range cc.GetMembers() {
			if m.GetIdentity().GetSelf() {
				return m.GetIdentity().GetId()
			}
		}
	}
	return ""
}

// selfRows counts the self-marked rows in one connection.
// Exactly one member served the response.
// A merge that forgot to clear self on the others would return more.
func selfRows(resp *proxyadminv1.DescribeClusterConnectionsResponse) int {
	n := 0
	for _, m := range resp.GetClusterConnections()[0].GetMembers() {
		if m.GetIdentity().GetSelf() {
			n++
		}
	}
	return n
}

// The peer listener exists for siblings.
// They only ever need one process's answer.
// Refusing to forward keeps a group call to a single round of fan-out.
func TestPeerListenerRefusesForwarding(t *testing.T) {
	peerA := dynamicAddress(t)
	proxyA, _ := buildPeerProxyAt(t, "pod-a", "conn-a", peerA, []string{peerA})
	require.NotNil(t, proxyA)

	client := adminClient(t, peerA)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := client.DescribeClusterConnections(
		metadata.AppendToOutgoingContext(ctx, adminplane.MDTarget, "conn-a"),
		&proxyadminv1.DescribeClusterConnectionsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))

	// A group scoped call is refused rather than quietly answered as a member.
	_, err = client.DescribeClusterConnections(
		metadata.AppendToOutgoingContext(ctx, adminplane.MDScope, "group"),
		&proxyadminv1.DescribeClusterConnectionsRequest{})
	require.Error(t, err)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// The listeners are optional.
// Absent configuration must produce none.
// Asserting only that would also hold if the feature were deleted.
// The enabled case is asserted alongside it.
func TestAdminListenersFollowConfiguration(t *testing.T) {
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	build := func(t *testing.T, admin config.ProxyAdminConfig) *Proxy {
		t.Helper()
		a := getDynamicPlccAddresses(t)
		proxy, err := NewProxy(config.NewMockConfigProvider(config.S2SProxyConfig{
			ProxyAdmin: admin,
			ClusterConnections: []config.ClusterConnConfig{
				makeMuxClusterConfig("conn", config.ConnTypeMuxServer, localFVI, remoteFVI,
					a.localProxyOutbound, a.localTemporalAddr, a.localProxyOutbound, a.localProxyInbound),
			},
		}), loggers, Identity{Version: "v-test", MemberID: "pod-a"})
		require.NoError(t, err)
		require.NoError(t, proxy.Start())
		t.Cleanup(proxy.Stop)
		return proxy
	}

	t.Run("none when unconfigured", func(t *testing.T) {
		require.Empty(t, build(t, config.ProxyAdminConfig{}).adminServers)
	})

	t.Run("operator only", func(t *testing.T) {
		proxy := build(t, config.ProxyAdminConfig{ListenAddress: dynamicAddress(t)})
		require.Len(t, proxy.adminServers, 1)
	})

	t.Run("operator and peer", func(t *testing.T) {
		peer := dynamicAddress(t)
		proxy := build(t, config.ProxyAdminConfig{
			ListenAddress: dynamicAddress(t),
			Peer:          &config.ProxyAdminPeerConfig{ListenAddress: peer},
		})
		require.Len(t, proxy.adminServers, 2)

		// A registered listener is one that actually answers.
		client := adminClient(t, peer)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := client.DescribeClusterConnections(ctx, &proxyadminv1.DescribeClusterConnectionsRequest{})
		require.NoError(t, err)
	})
}

// A diagnostic listener that cannot bind is unavailable, not a reason to refuse to proxy replication traffic.
// The peer listener on a free port proves the failure was contained to one.
func TestAdminBindFailureDoesNotStopStartup(t *testing.T) {
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	a := getDynamicPlccAddresses(t)

	taken := dynamicAddress(t)
	blocker, err := net.Listen("tcp", taken)
	require.NoError(t, err)
	defer func() { _ = blocker.Close() }()

	peer := dynamicAddress(t)
	proxy, err := NewProxy(config.NewMockConfigProvider(config.S2SProxyConfig{
		ProxyAdmin: config.ProxyAdminConfig{
			ListenAddress: taken,
			Peer:          &config.ProxyAdminPeerConfig{ListenAddress: peer},
		},
		ClusterConnections: []config.ClusterConnConfig{
			makeMuxClusterConfig("conn", config.ConnTypeMuxServer, localFVI, remoteFVI,
				a.localProxyOutbound, a.localTemporalAddr, a.localProxyOutbound, a.localProxyInbound),
		},
	}), loggers, Identity{Version: "v-test", MemberID: "pod-a"})
	require.NoError(t, err)
	require.NoError(t, proxy.Start())
	defer proxy.Stop()

	require.Len(t, proxy.adminServers, 1, "the listener that could not bind is skipped, the other still starts")
	require.True(t, proxy.clusterConnections[migrationId{"conn"}].AcceptingInboundTraffic(),
		"replication is unaffected by a diagnostic listener failing to bind")
}
