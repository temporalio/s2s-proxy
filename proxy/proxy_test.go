package proxy

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
)

type (
	proxyTestSuite struct {
		suite.Suite
		ctrl *gomock.Controller
	}

	proxyConfig struct {
		localServerAddress    string
		remoteServerAddress   string
		inboundServerAddress  string
		outboundServerAddress string
	}

	clusterInfo struct {
		serverAddress  string
		clusterShardID history.ClusterShardID
		proxyConfig    *proxyConfig
	}
)

func (pc *proxyConfig) GetGRPCServerOptions() []grpc.ServerOption {
	return nil
}

func (pc *proxyConfig) GetOutboundServerAddress() string {
	return pc.outboundServerAddress
}

func (pc *proxyConfig) GetInboundServerAddress() string {
	return pc.inboundServerAddress
}

func (pc *proxyConfig) GetRemoteServerRPCAddress() string {
	return pc.remoteServerAddress
}

func (pc *proxyConfig) GetLocalServerRPCAddress() string {
	return pc.localServerAddress
}

func TestProxyTestSuite(t *testing.T) {
	suite.Run(t, new(proxyTestSuite))
}

func (s *proxyTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
}

func (s *proxyTestSuite) AfterTest(suiteName, testName string) {
}

func verifyEcho(sequence []int64, echoed map[int64]bool) bool {
	if len(sequence) != len(echoed) {
		return false
	}

	for _, n := range sequence {
		if exists := echoed[n]; !exists {
			return false
		}
	}

	return true
}

func (s *proxyTestSuite) Test_NoProxy() {
	echoServerInfo := clusterInfo{
		serverAddress: "localhost:7266",
		clusterShardID: history.ClusterShardID{
			ClusterID: 1,
			ShardID:   2,
		},
		// proxyConfig: &proxyConfig{
		// 	inboundServerAddress:  "localhost:7366",
		// 	outboundServerAddress: "localhost:7466",
		// 	remoteServerAddress:   testTargetServerAddress,
		// 	localServerAddress:    testSourceServerAddress,
		// },
	}

	echoClientInfo := clusterInfo{
		clusterShardID: history.ClusterShardID{
			ClusterID: 2,
			ShardID:   4,
		},
	}

	logger := log.NewTestLogger()
	echoServer := newEchoServer(echoServerInfo.serverAddress, logger)
	echoServer.start()
	defer echoServer.stop()

	echoClient := newEchoClient(
		echoServerInfo.serverAddress,
		echoServerInfo.clusterShardID,
		echoClientInfo.clusterShardID,
		echoClientInfo.proxyConfig,
		logger,
	)

	sequence := []int64{1, 2, 3, 4, 5}
	echoed, err := echoClient.sendAndRecv(sequence)
	s.NoError(err)
	s.True(verifyEcho(sequence, echoed))
}

// func (s *proxyTestSuite) Test_StartStop() {
// 	sourceClusterInfo := clusterInfo{
// 		serverAddress: testSourceServerAddress,
// 		clusterShardID: history.ClusterShardID{
// 			ClusterID: 1,
// 			ShardID:   2,
// 		},
// 		proxyConfig: &proxyConfig{
// 			inboundServerAddress:  "localhost:7366",
// 			outboundServerAddress: "localhost:7466",
// 			remoteServerAddress:   testTargetServerAddress,
// 			localServerAddress:    testSourceServerAddress,
// 		},
// 	}

// 	targetClusterInfo := clusterInfo{
// 		serverAddress: testTargetServerAddress,
// 		clusterShardID: history.ClusterShardID{
// 			ClusterID: 2,
// 			ShardID:   4,
// 		},
// 	}

// 	logger := log.NewTestLogger()
// 	config := sourceClusterInfo.proxyConfig
// 	rpcFactory := rpc.NewRPCFactory(config, logger)
// 	clientFactory := client.NewClientFactory(rpcFactory, logger)
// 	proxy := NewProxy(
// 		config,
// 		logger,
// 		clientFactory,
// 	)

// 	sender := newProxyServer("sender", sourceClusterInfo.serverAddress, mockproxy.NewMockSenderService("sender", logger), nil, logger)
// 	sender.start()
// 	proxy.Start()

// 	defer func() {
// 		// proxy.Stop()
// 		// sender.stop()
// 	}()

// 	metaData := history.EncodeClusterShardMD(sourceClusterInfo.clusterShardID, targetClusterInfo.clusterShardID)
// 	targetConext := metadata.NewOutgoingContext(context.TODO(), metaData)
// 	remoteClient := clientFactory.NewRemoteAdminClient(config.inboundServerAddress)
// 	remoteServer, err := remoteClient.StreamWorkflowReplicationMessages(targetConext)
// 	s.NoError(err)

// 	highWatermarkInfo := &watermarkInfo{
// 		Watermark: 10,
// 	}

// 	req := &adminservice.StreamWorkflowReplicationMessagesRequest{
// 		Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
// 			SyncReplicationState: &repicationpb.SyncReplicationState{
// 				HighPriorityState: &repicationpb.ReplicationState{
// 					InclusiveLowWatermark:     highWatermarkInfo.Watermark,
// 					InclusiveLowWatermarkTime: timestamppb.New(highWatermarkInfo.Timestamp),
// 				},
// 			},
// 		}}

// 	remoteServer.Send(req)
// 	resp, err := remoteServer.Recv()
// 	s.NoError(err)
// 	_, ok := resp.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesResponse_Messages)
// 	s.True(ok)
// }
