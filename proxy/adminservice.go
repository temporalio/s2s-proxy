package proxy

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/client"
	adminclient "github.com/temporalio/s2s-proxy/client/admin"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	adminServiceProxyServer struct {
		adminservice.UnimplementedAdminServiceServer
		adminClient adminservice.AdminServiceClient
		logger      log.Logger
		proxyOptions
	}
)

func NewAdminServiceProxyServer(
	serviceName string,
	clientConfig config.ProxyClientConfig,
	clientFactory client.ClientFactory,
	opts proxyOptions,
	logger log.Logger,
) adminservice.AdminServiceServer {
	logger = log.With(logger, common.ServiceTag(serviceName))
	clientProvider := client.NewClientProvider(clientConfig, clientFactory, logger)
	return &adminServiceProxyServer{
		adminClient:  adminclient.NewLazyClient(clientProvider),
		logger:       logger,
		proxyOptions: opts,
	}
}

func (s *adminServiceProxyServer) AddOrUpdateRemoteCluster(ctx context.Context, in0 *adminservice.AddOrUpdateRemoteClusterRequest) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	if !common.IsRequestTranslationDisabled(ctx) {
		if outbound := s.Config.Outbound; s.IsInbound && outbound != nil && len(outbound.Server.ExternalAddress) > 0 {
			// Override this address so that cross-cluster connections flow through the proxy.
			// Use a separate "external address" config option because the outbound.listenerAddress may not be routable
			// from the local temporal server, or the proxy may be deployed behind a load balancer.
			in0.FrontendAddress = outbound.Server.ExternalAddress
		}
	}
	return s.adminClient.AddOrUpdateRemoteCluster(ctx, in0)
}

func (s *adminServiceProxyServer) AddSearchAttributes(ctx context.Context, in0 *adminservice.AddSearchAttributesRequest) (*adminservice.AddSearchAttributesResponse, error) {
	return s.adminClient.AddSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) AddTasks(ctx context.Context, in0 *adminservice.AddTasksRequest) (*adminservice.AddTasksResponse, error) {
	return s.adminClient.AddTasks(ctx, in0)
}

func (s *adminServiceProxyServer) CancelDLQJob(ctx context.Context, in0 *adminservice.CancelDLQJobRequest) (*adminservice.CancelDLQJobResponse, error) {
	return s.adminClient.CancelDLQJob(ctx, in0)
}

func (s *adminServiceProxyServer) CloseShard(ctx context.Context, in0 *adminservice.CloseShardRequest) (*adminservice.CloseShardResponse, error) {
	return s.adminClient.CloseShard(ctx, in0)
}

func (s *adminServiceProxyServer) DeleteWorkflowExecution(ctx context.Context, in0 *adminservice.DeleteWorkflowExecutionRequest) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	return s.adminClient.DeleteWorkflowExecution(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	resp, err := s.adminClient.DescribeCluster(ctx, in0)
	if common.IsRequestTranslationDisabled(ctx) {
		return resp, err
	}

	var overrides *config.APIOverridesConfig
	if s.IsInbound {
		if s.Config.Inbound != nil {
			overrides = s.Config.Inbound.APIOverrides
		}
	} else {
		if s.Config.Outbound != nil {
			overrides = s.Config.Outbound.APIOverrides
		}
	}

	if overrides != nil && overrides.AdminSerivce.DescribeCluster != nil {
		responseOverride := overrides.AdminSerivce.DescribeCluster.Response
		if resp != nil && responseOverride.FailoverVersionIncrement != nil {
			resp.FailoverVersionIncrement = *responseOverride.FailoverVersionIncrement
		}
	}

	return resp, err
}

func (s *adminServiceProxyServer) DescribeDLQJob(ctx context.Context, in0 *adminservice.DescribeDLQJobRequest) (*adminservice.DescribeDLQJobResponse, error) {
	return s.adminClient.DescribeDLQJob(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeHistoryHost(ctx context.Context, in0 *adminservice.DescribeHistoryHostRequest) (*adminservice.DescribeHistoryHostResponse, error) {
	return s.adminClient.DescribeHistoryHost(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeMutableState(ctx context.Context, in0 *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	return s.adminClient.DescribeMutableState(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQMessages(ctx context.Context, in0 *adminservice.GetDLQMessagesRequest) (*adminservice.GetDLQMessagesResponse, error) {
	return s.adminClient.GetDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQReplicationMessages(ctx context.Context, in0 *adminservice.GetDLQReplicationMessagesRequest) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	return s.adminClient.GetDLQReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetDLQTasks(ctx context.Context, in0 *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error) {
	return s.adminClient.GetDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) GetNamespace(ctx context.Context, in0 *adminservice.GetNamespaceRequest) (*adminservice.GetNamespaceResponse, error) {
	return s.adminClient.GetNamespace(ctx, in0)
}

func (s *adminServiceProxyServer) GetNamespaceReplicationMessages(ctx context.Context, in0 *adminservice.GetNamespaceReplicationMessagesRequest) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	return s.adminClient.GetNamespaceReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetReplicationMessages(ctx context.Context, in0 *adminservice.GetReplicationMessagesRequest) (*adminservice.GetReplicationMessagesResponse, error) {
	return s.adminClient.GetReplicationMessages(ctx, in0)
}

func (s *adminServiceProxyServer) GetSearchAttributes(ctx context.Context, in0 *adminservice.GetSearchAttributesRequest) (*adminservice.GetSearchAttributesResponse, error) {
	return s.adminClient.GetSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) GetShard(ctx context.Context, in0 *adminservice.GetShardRequest) (*adminservice.GetShardResponse, error) {
	return s.adminClient.GetShard(ctx, in0)
}

func (s *adminServiceProxyServer) GetTaskQueueTasks(ctx context.Context, in0 *adminservice.GetTaskQueueTasksRequest) (*adminservice.GetTaskQueueTasksResponse, error) {
	return s.adminClient.GetTaskQueueTasks(ctx, in0)
}

func (s *adminServiceProxyServer) GetWorkflowExecutionRawHistory(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryRequest) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	return s.adminClient.GetWorkflowExecutionRawHistory(ctx, in0)
}

func (s *adminServiceProxyServer) GetWorkflowExecutionRawHistoryV2(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryV2Request) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	return s.adminClient.GetWorkflowExecutionRawHistoryV2(ctx, in0)
}

func (s *adminServiceProxyServer) ImportWorkflowExecution(ctx context.Context, in0 *adminservice.ImportWorkflowExecutionRequest) (*adminservice.ImportWorkflowExecutionResponse, error) {
	return s.adminClient.ImportWorkflowExecution(ctx, in0)
}

func (s *adminServiceProxyServer) ListClusterMembers(ctx context.Context, in0 *adminservice.ListClusterMembersRequest) (*adminservice.ListClusterMembersResponse, error) {
	return s.adminClient.ListClusterMembers(ctx, in0)
}

func (s *adminServiceProxyServer) ListClusters(ctx context.Context, in0 *adminservice.ListClustersRequest) (*adminservice.ListClustersResponse, error) {
	return s.adminClient.ListClusters(ctx, in0)
}

func (s *adminServiceProxyServer) ListHistoryTasks(ctx context.Context, in0 *adminservice.ListHistoryTasksRequest) (*adminservice.ListHistoryTasksResponse, error) {
	return s.adminClient.ListHistoryTasks(ctx, in0)
}

func (s *adminServiceProxyServer) ListQueues(ctx context.Context, in0 *adminservice.ListQueuesRequest) (*adminservice.ListQueuesResponse, error) {
	return s.adminClient.ListQueues(ctx, in0)
}

func (s *adminServiceProxyServer) MergeDLQMessages(ctx context.Context, in0 *adminservice.MergeDLQMessagesRequest) (*adminservice.MergeDLQMessagesResponse, error) {
	return s.adminClient.MergeDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) MergeDLQTasks(ctx context.Context, in0 *adminservice.MergeDLQTasksRequest) (*adminservice.MergeDLQTasksResponse, error) {
	return s.adminClient.MergeDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) PurgeDLQMessages(ctx context.Context, in0 *adminservice.PurgeDLQMessagesRequest) (*adminservice.PurgeDLQMessagesResponse, error) {
	return s.adminClient.PurgeDLQMessages(ctx, in0)
}

func (s *adminServiceProxyServer) PurgeDLQTasks(ctx context.Context, in0 *adminservice.PurgeDLQTasksRequest) (*adminservice.PurgeDLQTasksResponse, error) {
	return s.adminClient.PurgeDLQTasks(ctx, in0)
}

func (s *adminServiceProxyServer) ReapplyEvents(ctx context.Context, in0 *adminservice.ReapplyEventsRequest) (*adminservice.ReapplyEventsResponse, error) {
	return s.adminClient.ReapplyEvents(ctx, in0)
}

func (s *adminServiceProxyServer) RebuildMutableState(ctx context.Context, in0 *adminservice.RebuildMutableStateRequest) (*adminservice.RebuildMutableStateResponse, error) {
	return s.adminClient.RebuildMutableState(ctx, in0)
}

func (s *adminServiceProxyServer) RefreshWorkflowTasks(ctx context.Context, in0 *adminservice.RefreshWorkflowTasksRequest) (*adminservice.RefreshWorkflowTasksResponse, error) {
	return s.adminClient.RefreshWorkflowTasks(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveRemoteCluster(ctx context.Context, in0 *adminservice.RemoveRemoteClusterRequest) (*adminservice.RemoveRemoteClusterResponse, error) {
	return s.adminClient.RemoveRemoteCluster(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveSearchAttributes(ctx context.Context, in0 *adminservice.RemoveSearchAttributesRequest) (*adminservice.RemoveSearchAttributesResponse, error) {
	return s.adminClient.RemoveSearchAttributes(ctx, in0)
}

func (s *adminServiceProxyServer) RemoveTask(ctx context.Context, in0 *adminservice.RemoveTaskRequest) (*adminservice.RemoveTaskResponse, error) {
	return s.adminClient.RemoveTask(ctx, in0)
}

func (s *adminServiceProxyServer) ResendReplicationTasks(ctx context.Context, in0 *adminservice.ResendReplicationTasksRequest) (*adminservice.ResendReplicationTasksResponse, error) {
	return s.adminClient.ResendReplicationTasks(ctx, in0)
}

func ClusterShardIDtoString(sd history.ClusterShardID) string {
	return fmt.Sprintf("(id: %d, shard: %d)", sd.ClusterID, sd.ShardID)
}

// StreamWorkflowReplicationMessages establishes an HTTP/2 stream. gRPC passes us a stream that represents the initiating server,
// and we can freely Send and Recv on that "server". Because this is a proxy, we also establish a bidirectional
// stream using our configured adminClient. When we Recv on the initiator, we Send to the client.
// When we Recv on the client, we Send to the initiator
func (s *adminServiceProxyServer) StreamWorkflowReplicationMessages(
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
) (retError error) {
	defer log.CapturePanic(s.logger, &retError)

	targetMetadata, ok := metadata.FromIncomingContext(targetStreamServer.Context())
	if !ok {
		return serviceerror.NewInvalidArgument("missing cluster & shard ID metadata")
	}
	targetClusterShardID, sourceClusterShardID, err := history.DecodeClusterShardMD(
		headers.NewGRPCHeaderGetter(targetStreamServer.Context()),
	)
	if err != nil {
		return err
	}

	logger := log.With(s.logger,
		tag.NewStringTag("source", ClusterShardIDtoString(sourceClusterShardID)),
		tag.NewStringTag("target", ClusterShardIDtoString(targetClusterShardID)))

	// Record streams active
	directionLabel := "inbound"
	if !s.IsInbound {
		directionLabel = "outbound"
	}
	logger.Info("AdminStreamReplicationMessages started.")
	streamsActiveGauge := metrics.AdminServiceStreamsActive.WithLabelValues(directionLabel)
	streamsActiveGauge.Inc()
	metrics.AdminServiceStreamsOpenedCount.WithLabelValues(directionLabel).Inc()
	defer streamsActiveGauge.Dec()
	defer logger.Info("AdminStreamReplicationMessages stopped.")

	// simply forwarding target metadata
	outgoingContext := metadata.NewOutgoingContext(targetStreamServer.Context(), targetMetadata)
	outgoingContext, cancel := context.WithCancel(outgoingContext)
	defer cancel()

	sourceStreamClient, err := s.adminClient.StreamWorkflowReplicationMessages(outgoingContext)
	if err != nil {
		logger.Error("remoteAdminServiceClient.StreamWorkflowReplicationMessages encountered error", tag.Error(err))
		return err
	}
	streamStartTime := time.Now()

	// When one side of the stream dies, we want to tell the other side to hang up
	// (see https://stackoverflow.com/questions/68218469/how-to-un-wedge-go-grpc-bidi-streaming-server-from-the-blocking-recv-call)
	// One call to StreamWorkflowReplicationMessages establishes a one-way channel through the proxy from one server to another.
	// For an outbound stream, we have:
	// StreamWorkflowReplicationMessagesRequest
	// Local.Recv ===> Proxy ===> Remote.Send
	//  ^targetStreamServer    ^sourceStreamClient
	//
	// StreamWorkflowReplicationMessagesResponse
	// Local.Send <=== Proxy <=== Remote.Recv
	//  ^targetStreamServer   ^sourceStreamClient
	//
	// We can freely close sourceStreamClient with closeSend. gRPC will only cancel targetStreamServer when we return from
	// this function.
	// Scenario 1: Remote disconnects. sourceStreamClient.Recv will return EOF. This unblocks transferSourceToTarget and sets shutdownChan.
	//             transferTargetToSource needs to be unblocked from targetStreamServer.Recv
	// Scenario 2: Local disconnects. targetStreamServer.Recv will return EOF. This unblocks transferTargetToSource and sets shutdownChan.
	//             transferSourceToTarget needs to be unblocked from sourceStreamClient.Recv
	shutdownChan := channel.NewShutdownOnce()
	sendEOFToServer := atomic.Bool{}
	var wg sync.WaitGroup
	wg.Add(2)
	go transferTargetToSource(sourceStreamClient, targetStreamServer, &wg, shutdownChan, &sendEOFToServer, directionLabel, logger)
	go transferSourceToTarget(sourceStreamClient, targetStreamServer, &wg, shutdownChan, &sendEOFToServer, directionLabel, logger)
	wg.Wait()

	streamDuration := time.Since(streamStartTime)
	metrics.AdminServiceStreamDuration.WithLabelValues(directionLabel).Observe(streamDuration.Seconds())
	metrics.AdminServiceStreamsClosedCount.WithLabelValues(directionLabel).Inc()
	if sendEOFToServer.Load() {
		return io.EOF
	}
	return nil
}
