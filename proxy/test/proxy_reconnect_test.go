package proxy

import (
	"time"

	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
)

var (
	emptyReq = &adminservice.StreamWorkflowReplicationMessagesRequest{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
			SyncReplicationState: &replicationpb.SyncReplicationState{
				HighPriorityState: &replicationpb.ReplicationState{},
			},
		}}
)

func (s *proxyTestSuite) Test_ForceStopSourceServer() {
	logger := log.NewTestLogger()

	echoServerInfo := clusterInfo{
		serverAddress:  echoServerAddress,
		clusterShardID: serverClusterShard,
	}

	echoClientInfo := clusterInfo{
		serverAddress:  echoClientAddress,
		clusterShardID: clientClusterShard,
		s2sProxyConfig: createEchoClientConfig(),
	}

	echoServer := newEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := newEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoServer.start()
	echoClient.start()

	stream, err := echoClient.CreateStreamClient()
	s.NoError(err)
	_, err = sendRecv(stream, []int64{1})
	s.NoError(err)

	echoServer.server.ForceStop()

	// ForceStop cause sourceStreamClient.Recv in Upstream loop within
	// StreamWorkflowReplicationMessages handler to fail. Wait for
	// StreamWorkflowReplicationMessages handler returns, which stop
	// Downstream loop.
	time.Sleep(time.Second)

	err = stream.Send(emptyReq)

	// This should fail because StreamWorkflowReplicationMessages handler stopped.
	s.ErrorContains(err, "EOF")

	stream.CloseSend()
	echoClient.stop()
}
