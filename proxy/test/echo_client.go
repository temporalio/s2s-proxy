package proxy

import (
	"context"
	"fmt"
	"time"

	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/gogo/status"
	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

type (
	echoClient struct {
		echoServerClusterShard history.ClusterShardID
		echoClientClusterShard history.ClusterShardID
		serviceName            string
		logger                 log.Logger
		proxy                  *s2sproxy.Proxy
		clientProvider         client.ClientProvider
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
	localClusterInfo clusterInfo,
	remoteClusterInfo clusterInfo,
	logger log.Logger,
) *echoClient {
	rpcFactory := rpc.NewRPCFactory(&mockConfigProvider{}, logger)
	clientFactory := client.NewClientFactory(rpcFactory, logger)

	var proxy *s2sproxy.Proxy
	var err error
	var clientConfig config.ClientConfig

	if localClusterInfo.s2sProxyConfig != nil {
		// Setup EchoClient's proxy and connect EchoClient to the proxy (via outbound server).
		if remoteClusterInfo.s2sProxyConfig != nil {
			// remote.proxy.Inbound	<- - -> local.proxy <-> EchoClient
			localClusterInfo.s2sProxyConfig.Outbound.Client.ForwardAddress = remoteClusterInfo.s2sProxyConfig.Inbound.Server.ListenAddress
		} else {
			// remote.server <- - -> local.proxy <-> EchoClient
			localClusterInfo.s2sProxyConfig.Outbound.Client.ForwardAddress = remoteClusterInfo.serverAddress
		}

		configProvider := &mockConfigProvider{
			proxyConfig: *localClusterInfo.s2sProxyConfig,
		}
		proxy, err = s2sproxy.NewProxy(configProvider, logger, clientFactory)
		if err != nil {
			logger.Fatal("Failed to create proxy")
		}
		clientConfig = config.ClientConfig{
			ForwardAddress: localClusterInfo.s2sProxyConfig.Outbound.Server.ListenAddress,
		}
	} else if remoteClusterInfo.s2sProxyConfig != nil {
		// 	remote.proxy.Inbound <- - -> EchoClient
		clientConfig = config.ClientConfig{
			ForwardAddress: remoteClusterInfo.s2sProxyConfig.Inbound.Server.ListenAddress,
		}
	} else {
		// 	remote.server <- - -> EchoClient
		clientConfig = config.ClientConfig{
			ForwardAddress: remoteClusterInfo.serverAddress,
		}
	}

	logger = log.With(logger, common.ServiceTag("EchoClient"), tag.NewAnyTag("clientForwardAddress", clientConfig.ForwardAddress))
	return &echoClient{
		echoServerClusterShard: remoteClusterInfo.clusterShardID,
		echoClientClusterShard: localClusterInfo.clusterShardID,
		serviceName:            "EchoClient",
		logger:                 logger,
		proxy:                  proxy,
		clientProvider:         client.NewClientProvider(clientConfig, clientFactory, logger),
	}
}

func (r *echoClient) start() {
	r.logger.Info(fmt.Sprintf("Starting %s", r.serviceName))
	if r.proxy != nil {
		_ = r.proxy.Start()
	}
}

func (r *echoClient) stop() {
	r.logger.Info(fmt.Sprintf("Stopping %s", r.serviceName))
	if r.proxy != nil {
		r.proxy.Stop()
	}
}

const (
	retryInterval = 1 * time.Second // Interval between retries
)

func retry[T interface{}](f func() (T, error), maxRetries int, logger log.Logger) (T, error) {
	var err error
	var output T
	for i := 0; i < maxRetries; i++ {
		output, err = f()
		if err != nil {
			// Check if the error is a gRPC Unavailable error
			if status.Code(err) == codes.Unavailable {
				logger.Warn("Retry due to Unavailable error", tag.Error(err))
				time.Sleep(retryInterval)
				continue
			}

			return output, err
		}

		return output, nil
	}

	return output, fmt.Errorf("failed to call method after %d retries: %w", maxRetries, err)
}

// Send a sequence of numbers and return which numbers have been echoed back.
func (r *echoClient) sendAndRecv(sequence []int64) (map[int64]bool, error) {
	echoed := make(map[int64]bool)
	metaData := history.EncodeClusterShardMD(r.echoClientClusterShard, r.echoServerClusterShard)
	targetContext := metadata.NewOutgoingContext(context.TODO(), metaData)

	adminClient, err := r.clientProvider.GetAdminClient()
	if err != nil {
		return echoed, err
	}

	stream, err := retry(func() (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
		return adminClient.StreamWorkflowReplicationMessages(targetContext)
	}, 5, r.logger)
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

	_ = stream.CloseSend()
	r.logger.Info("==== sendAndRecv completed ====")
	return echoed, nil
}
