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

func (s *proxyTestSuite) Test_RestartStreamServer() {
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
	_, err = echo(stream, []int64{1})
	s.NoError(err)

	echoServer.server.ForceStop()

	time.Sleep(time.Second)

	err = stream.Send(emptyReq)
	s.ErrorContains(err, "EOF")
	stream.CloseSend()
	echoClient.stop()
}
