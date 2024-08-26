package proxy

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/status"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/encryption"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

type (
	echoClient struct {
		adminClient            adminservice.AdminServiceClient
		echoServerClusterShard history.ClusterShardID
		echoClientClusterShard history.ClusterShardID
		serviceName            string
		logger                 log.Logger
		proxy                  *s2sproxy.Proxy
	}

	watermarkInfo struct {
		Watermark int64
		Timestamp time.Time
	}
)

// Echo client for testing stream replication. It acts as stream receiver.
// It starts a bi-directional stream by connecting to Echo server (which acts as stream sender).
// It sends a sequence of numbers as SyncReplicationState message and then wait for Echo server
// to reply.
func newEchoClient(
	serverInfo clusterInfo,
	clientInfo clusterInfo,
	logger log.Logger,
) *echoClient {
	rpcFactory := rpc.NewRPCFactory(clientInfo.proxyConfig, logger)
	tlsConfigProvider := encryption.NewTLSConfigProfilder(logger)
	clientFactory := client.NewClientFactory(rpcFactory, tlsConfigProvider, logger)

	var proxy *s2sproxy.Proxy
	var adminClient adminservice.AdminServiceClient

	var emptyConfig encryption.ClientTLSConfig

	if clientInfo.proxyConfig != nil {
		// Setup EchoClient's proxy and connect EchoClient to the proxy (via outbound server).
		// 	<- - -> proxy <-> EchoClient
		proxy = s2sproxy.NewProxy(clientInfo.proxyConfig, logger, clientFactory)
		adminClient = clientFactory.NewRemoteAdminClient(clientInfo.proxyConfig.GetOutboundServerAddress(), emptyConfig)
	} else if serverInfo.proxyConfig != nil {
		// Connect EchoClient to EchoServer's proxy (via InboundServer).
		// 	EchoServer <-> proxy <- - -> EchoClient
		adminClient = clientFactory.NewRemoteAdminClient(serverInfo.proxyConfig.GetInboundServerAddress(), emptyConfig)
	} else {
		// Connect EchoClient directly to EchoServer.
		// 	EchoServer <- - -> EchoClient
		adminClient = clientFactory.NewRemoteAdminClient(serverInfo.serverAddress, emptyConfig)
	}

	return &echoClient{
		adminClient:            adminClient,
		echoServerClusterShard: serverInfo.clusterShardID,
		echoClientClusterShard: clientInfo.clusterShardID,
		serviceName:            "EchoClient",
		logger:                 log.With(logger, common.ServiceTag("EchoClient")),
		proxy:                  proxy,
	}
}

func (r *echoClient) start() {
	if r.proxy != nil {
		r.proxy.Start()
	}
}

func (r *echoClient) stop() {
	if r.proxy != nil {
		r.proxy.Stop()
	}
}

const (
	retryInterval = 1 * time.Second // Interval between retries
)

// Retry in case that EchoServer or Proxy might be ready when calling stream API.
func (r *echoClient) retryStreamWorkflowReplicationMessages(maxRetries int) (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
	var lastErr error
	metaData := history.EncodeClusterShardMD(r.echoClientClusterShard, r.echoServerClusterShard)
	targetConext := metadata.NewOutgoingContext(context.TODO(), metaData)

	for i := 0; i < maxRetries; i++ {
		stream, err := r.adminClient.StreamWorkflowReplicationMessages(targetConext)
		if err != nil {
			lastErr = err
			// Check if the error is a gRPC Unavailable error
			if status.Code(err) == codes.Unavailable {
				r.logger.Warn("Retry StreamWorkflowReplicationMessages due to Unavailable error", tag.Error(err))
				time.Sleep(retryInterval)
				continue
			}

			return nil, err
		}

		return stream, nil
	}

	return nil, fmt.Errorf("failed to establish stream after %d retries: %w", maxRetries, lastErr)
}

// Send a sequence of numbers and return which numbers have been echoed back.
func (r *echoClient) sendAndRecv(sequence []int64) (map[int64]bool, error) {
	echoed := make(map[int64]bool)
	stream, err := r.retryStreamWorkflowReplicationMessages(5)
	if err != nil {
		return echoed, err
	}

	r.logger.Info("==== sendAndRecv starting ====")

	for _, waterMark := range sequence {
		highWatermarkInfo := &watermarkInfo{
			Watermark: waterMark,
		}

		req := &adminservice.StreamWorkflowReplicationMessagesRequest{
			Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
				SyncReplicationState: &replicationpb.SyncReplicationState{
					HighPriorityState: &replicationpb.ReplicationState{
						InclusiveLowWatermark:     highWatermarkInfo.Watermark,
						InclusiveLowWatermarkTime: timestamppb.New(highWatermarkInfo.Timestamp),
					},
				},
			}}

		if err = stream.Send(req); err != nil {
			return echoed, err
		}
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
	r.logger.Info("==== sendAndRecv completed ====")
	return echoed, nil
}
