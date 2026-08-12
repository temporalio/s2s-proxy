package proxy

import (
	"time"

	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/adminplane"
	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
	"github.com/temporalio/s2s-proxy/endtoendtest"
)

func connectionState(desc *proxyadminv1.DescribeClusterConnectionsResponse) proxyadminv1.ConnectionState {
	conns := desc.GetClusterConnections()
	// there must only be a single connection - otherwise could be misleading.
	if len(conns) != 1 {
		return proxyadminv1.ConnectionState_CONNECTION_STATE_UNSPECIFIED
	}
	return conns[0].GetState()
}

// soleConnName returns "" if there is not exactly one cluster connection.
func soleConnName(desc *proxyadminv1.DescribeClusterConnectionsResponse) string {
	conns := desc.GetClusterConnections()
	if len(conns) != 1 {
		return ""
	}
	return conns[0].GetName()
}

func (s *proxyTestSuite) Test_SelfClusterConnections_MuxStateFlips() {
	// echoServer dials, as the mux client.
	// echoClient listens, as the mux server.
	echoServerConfig := s.createEchoServerConfig(
		withRemoteMuxClient(s.clientProxyInboundAddress),
	)
	echoClientConfig := s.createEchoClientConfig(
		withRemoteMuxServer(s.clientProxyInboundAddress),
	)

	echoServerInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoServerAddress,
		ClusterShardID: serverClusterShard,
		S2sProxyConfig: echoServerConfig,
	}
	echoClientInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoClientAddress,
		ClusterShardID: clientClusterShard,
		S2sProxyConfig: echoClientConfig,
	}

	logger := log.NewTestLogger()
	echoServer := endtoendtest.NewEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := endtoendtest.NewEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	proxy := echoClient.Proxy
	ctx := s.T().Context()

	isConnected := func() bool {
		return connectionState(proxy.SelfClusterConnections(ctx)) ==
			proxyadminv1.ConnectionState_CONNECTION_STATE_CONNECTED
	}

	// connectionState assumes one connection. Assert that shape once, here.
	s.Require().Len(proxy.SelfClusterConnections(ctx).GetClusterConnections(), 1)

	// 1. Constructed, nothing started.
	s.Equal(proxyadminv1.ConnectionState_CONNECTION_STATE_ERROR,
		connectionState(proxy.SelfClusterConnections(ctx)))

	// 2. Mux listener up. Nobody has dialed it yet.
	echoClient.Start()
	defer echoClient.Stop()

	s.Equal(proxyadminv1.ConnectionState_CONNECTION_STATE_ERROR,
		connectionState(proxy.SelfClusterConnections(ctx)), "no peer has dialed yet")

	// 3. Peer dials in: ERROR -> CONNECTED.
	echoServer.Start()
	defer echoServer.Stop()

	s.Eventually(isConnected, 10*time.Second, 100*time.Millisecond,
		"state never flipped to CONNECTED")

	// 4. Peer goes away: CONNECTED -> not CONNECTED.
	echoServer.Stop()

	s.Eventually(func() bool { return !isConnected() }, 10*time.Second, 100*time.Millisecond,
		"state never flipped away from CONNECTED")
}

// Test_DescribeClusterConnections_OverLocalListener covers the operator endpoint end to end,
// through a real listener so that the interceptor which resolves scope and target actually runs.
//
// echoServer's connection is named proxy1 and reaches echoClient, named proxy2. Getting "proxy2"
// back proves the call crossed the mux and was answered by the other proxy.
func (s *proxyTestSuite) Test_DescribeClusterConnections_OverLocalListener() {
	adminAddress := GetLocalhostAddress()

	echoServerInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoServerAddress,
		ClusterShardID: serverClusterShard,
		S2sProxyConfig: s.createEchoServerConfig(
			withRemoteMuxClient(s.clientProxyInboundAddress),
			withProxyAdmin(adminAddress),
		),
	}
	echoClientInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoClientAddress,
		ClusterShardID: clientClusterShard,
		S2sProxyConfig: s.createEchoClientConfig(withRemoteMuxServer(s.clientProxyInboundAddress)),
	}

	logger := log.NewTestLogger()
	echoServer := endtoendtest.NewEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := endtoendtest.NewEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoClient.Start()
	defer echoClient.Stop()
	echoServer.Start()
	defer echoServer.Stop()

	conn, err := grpc.NewClient(adminAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	s.Require().NoError(err)
	defer func() { _ = conn.Close() }()
	client := proxyadminv1.NewProxyAdminServiceClient(conn)
	ctx := s.T().Context()
	empty := &proxyadminv1.DescribeClusterConnectionsRequest{}

	// No target: this proxy answers for its own deployment.
	self, err := client.DescribeClusterConnections(ctx, empty)
	s.Require().NoError(err)
	s.Equal("proxy1", soleConnName(self))
	// The literal, not MemberID().
	// MemberID() would compare the field against itself and pass whichever proxy answered.
	s.Equal("EchoServer", selfMemberID(self), "the proxy that received the call answers for itself")

	// A session that never established is not in the manager.
	// Sessions connected reads as healthy on its own.
	// The target is what an operator compares it against.
	selfConn := self.GetClusterConnections()[0]
	s.Positive(selfConn.GetMuxSessionsTarget(), "the configured session count is reported")
	s.Equal(selfConn.GetMuxSessionsTarget(), selfConn.GetMembers()[0].GetMuxSessionsTarget())

	// Member scope: the same answer, from one process.
	member, err := client.DescribeClusterConnections(
		metadata.AppendToOutgoingContext(ctx, adminplane.MDScope, "member"), empty)
	s.Require().NoError(err)
	s.Equal("proxy1", soleConnName(member))
	s.Len(member.GetClusterConnections()[0].GetMembers(), 1, "member scope describes one process")

	// Target set: the peer proxy answers, for its own deployment.
	targeted := metadata.AppendToOutgoingContext(ctx, adminplane.MDTarget, "proxy1")
	s.Eventually(func() bool {
		resp, err := client.DescribeClusterConnections(targeted, empty)
		return err == nil && soleConnName(resp) == "proxy2"
	}, 10*time.Second, 100*time.Millisecond, "the local listener never returned the peer proxy's answer")

	// An unknown target names a connection that does not exist.
	// That is a caller mistake.
	_, err = client.DescribeClusterConnections(
		metadata.AppendToOutgoingContext(ctx, adminplane.MDTarget, "nope"), empty)
	s.Require().Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))

	// An unrecognized scope is refused rather than silently answered at a narrower one.
	// The caller could not distinguish a narrowed answer from a real one.
	_, err = client.DescribeClusterConnections(
		metadata.AppendToOutgoingContext(ctx, adminplane.MDScope, "galaxy"), empty)
	s.Require().Error(err)
	s.Equal(codes.InvalidArgument, status.Code(err))
}

// A peer cluster reaches the admin service over the mux. It must see only the connection its call
// arrived on, never the other migrations this proxy carries.
func (s *proxyTestSuite) Test_DescribeClusterConnections_CounterpartyIsScopedToItsConnection() {
	echoServerInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoServerAddress,
		ClusterShardID: serverClusterShard,
		S2sProxyConfig: s.createEchoServerConfig(withRemoteMuxClient(s.clientProxyInboundAddress)),
	}
	echoClientInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoClientAddress,
		ClusterShardID: clientClusterShard,
		S2sProxyConfig: s.createEchoClientConfig(withRemoteMuxServer(s.clientProxyInboundAddress)),
	}

	logger := log.NewTestLogger()
	echoServer := endtoendtest.NewEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := endtoendtest.NewEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoClient.Start()
	defer echoClient.Stop()
	echoServer.Start()
	defer echoServer.Stop()

	// PeerConn reaches the far proxy's mux-registered service.
	// That is the counterparty listener.
	var client proxyadminv1.ProxyAdminServiceClient
	s.Eventually(func() bool {
		cc, err := echoServer.Proxy.PeerConn("proxy1")
		if err != nil {
			return false
		}
		client = proxyadminv1.NewProxyAdminServiceClient(cc)
		return true
	}, 10*time.Second, 100*time.Millisecond, "no mux session to the peer was ever established")

	ctx := s.T().Context()
	resp, err := client.DescribeClusterConnections(ctx, &proxyadminv1.DescribeClusterConnectionsRequest{})
	s.Require().NoError(err)
	s.Equal("proxy2", soleConnName(resp), "a counterparty sees only the connection it arrived on")

	// Forwarding is refused: otherwise a peer could pivot through us into a third cluster.
	_, err = client.DescribeClusterConnections(
		metadata.AppendToOutgoingContext(ctx, adminplane.MDTarget, "proxy2"),
		&proxyadminv1.DescribeClusterConnectionsRequest{})
	s.Require().Error(err)
	s.Equal(codes.PermissionDenied, status.Code(err))
}

// selfMemberID returns the id of the pod that served a response.
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
