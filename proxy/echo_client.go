package proxy

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/server/api/adminservice/v1"
	repicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/config"
)

type (
	echoClient struct {
		adminClient            adminservice.AdminServiceClient
		echoServerClusterShard history.ClusterShardID
		echoClientClusterShard history.ClusterShardID
	}

	watermarkInfo struct {
		Watermark int64
		Timestamp time.Time
	}
)

func newEchoClient(
	echoServerAddress string,
	serverClusterID history.ClusterShardID,
	clientClusterID history.ClusterShardID,
	config config.Config,
	logger log.Logger,
) *echoClient {
	rpcFactory := rpc.NewRPCFactory(config, logger)
	clientFactory := client.NewClientFactory(rpcFactory, logger)
	return &echoClient{
		adminClient:            clientFactory.NewRemoteAdminClient(echoServerAddress),
		echoServerClusterShard: serverClusterID,
		echoClientClusterShard: clientClusterID,
	}
}

func (r *echoClient) sendAndRecv(sequence []int64) (map[int64]bool, error) {
	metaData := history.EncodeClusterShardMD(r.echoServerClusterShard, r.echoClientClusterShard)
	targetConext := metadata.NewOutgoingContext(context.TODO(), metaData)
	stream, err := r.adminClient.StreamWorkflowReplicationMessages(targetConext)
	echoed := make(map[int64]bool)

	if err != nil {
		return echoed, err
	}

	for _, waterMark := range sequence {
		highWatermarkInfo := &watermarkInfo{
			Watermark: waterMark,
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

		stream.Send(req)
	}

	for i := 0; i < len(sequence); i++ {
		resp, err := stream.Recv()
		if err != nil {
			return echoed, err
		}

		switch attr := resp.GetAttributes().(type) {
		case *adminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			waterMark := attr.Messages.ExclusiveHighWatermark
			echoed[waterMark] = true
		default:
			return echoed, fmt.Errorf("sourceStreamClient.Recv encountered error")
		}
	}

	stream.CloseSend()
	return echoed, nil
}
