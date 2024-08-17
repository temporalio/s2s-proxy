package proxy

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/api/adminservice/v1"
	repicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/client/rpc"
	mockproxy "github.com/temporalio/s2s-proxy/mocks/proxy"
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

	watermarkInfo struct {
		Watermark int64
		Timestamp time.Time
	}

	clusterInfo struct {
		serverAddress  string
		clusterShardID history.ClusterShardID
		proxyConfig    *proxyConfig
	}
)

const (
	testSourceServerAddress = "localhost:7266"
	testTargetServerAddress = "localhost:8266"
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

func (s *proxyTestSuite) Test_StartStop() {
	sourceClusterInfo := clusterInfo{
		serverAddress: testSourceServerAddress,
		clusterShardID: history.ClusterShardID{
			ClusterID: 1,
			ShardID:   2,
		},
		proxyConfig: &proxyConfig{
			inboundServerAddress:  "localhost:7366",
			outboundServerAddress: "localhost:7466",
			remoteServerAddress:   testTargetServerAddress,
			localServerAddress:    testSourceServerAddress,
		},
	}

	targetClusterInfo := clusterInfo{
		serverAddress: testTargetServerAddress,
		clusterShardID: history.ClusterShardID{
			ClusterID: 2,
			ShardID:   4,
		},
	}

	logger := log.NewTestLogger()
	config := sourceClusterInfo.proxyConfig
	rpcFactory := rpc.NewRPCFactory(config, logger)
	clientFactory := client.NewClientFactory(rpcFactory, logger)
	proxy := NewProxy(
		config,
		logger,
		clientFactory,
	)

	sender := newProxyServer("sender", testSourceServerAddress, mockproxy.NewMockSenderService("sender", logger), nil, logger)
	sender.start()
	proxy.Start()

	defer func() {
		// proxy.Stop()
		// sender.stop()
	}()

	metaData := history.EncodeClusterShardMD(sourceClusterInfo.clusterShardID, targetClusterInfo.clusterShardID)
	targetConext := metadata.NewOutgoingContext(context.TODO(), metaData)
	remoteClient := clientFactory.NewRemoteAdminClient(config.inboundServerAddress)
	remoteServer, err := remoteClient.StreamWorkflowReplicationMessages(targetConext)
	s.NoError(err)

	highWatermarkInfo := &watermarkInfo{
		Watermark: 10,
	}

	req := &adminservice.StreamWorkflowReplicationMessagesRequest{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
			SyncReplicationState: &repicationpb.SyncReplicationState{
				HighPriorityState: &repicationpb.ReplicationState{
					InclusiveLowWatermark:     highWatermarkInfo.Watermark,
					InclusiveLowWatermarkTime: timestamppb.New(highWatermarkInfo.Timestamp),
				},
			},
		}}

	remoteServer.Send(req)
	resp, err := remoteServer.Recv()
	s.NoError(err)
	_, ok := resp.GetAttributes().(*adminservice.StreamWorkflowReplicationMessagesResponse_Messages)
	s.True(ok)
}
