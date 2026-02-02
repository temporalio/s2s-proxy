package proxy

import (
	"context"
	"fmt"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	LCMParameters struct {
		LCM              int32
		TargetShardCount int32
	}

	RoutingParameters struct {
		OverrideShardCount     int32
		RoutingLocalShardCount int32
		DirectionLabel         string
	}

	adminServiceProxyServer struct {
		adminservice.UnimplementedAdminServiceServer
		shardManager       ShardManager
		adminClient        adminservice.AdminServiceClient
		adminClientReverse adminservice.AdminServiceClient
		loggers            logging.LoggerProvider
		overrides          AdminServiceOverrides
		metricLabelValues  []string
		reportStreamValue  func(idx int32, value int32)
		shardCountConfig   config.ShardCountConfig
		lcmParameters      LCMParameters
		routingParameters  RoutingParameters
		lifetime           context.Context
	}
	AdminServiceOverrides struct {
		// Failover Version Increment. Overrides the value returned by DescribeCluster
		FVI                 int64
		ReplicationEndpoint string
	}
)

// NewAdminServiceProxyServer creates a proxy admin service.
func NewAdminServiceProxyServer(
	serviceName string,
	adminClient adminservice.AdminServiceClient,
	adminClientReverse adminservice.AdminServiceClient,
	overrides AdminServiceOverrides,
	metricLabelValues []string,
	reportStreamValue func(idx int32, value int32),
	shardCountConfig config.ShardCountConfig,
	lcmParameters LCMParameters,
	routingParameters RoutingParameters,
	logProvider logging.LoggerProvider,
	shardManager ShardManager,
	lifetime context.Context,
) adminservice.AdminServiceServer {
	return &adminServiceProxyServer{
		shardManager:       shardManager,
		adminClient:        adminClient,
		adminClientReverse: adminClientReverse,
		loggers:            logProvider.With(common.ServiceTag(serviceName)),
		overrides:          overrides,
		metricLabelValues:  metricLabelValues,
		reportStreamValue:  reportStreamValue,
		shardCountConfig:   shardCountConfig,
		lcmParameters:      lcmParameters,
		routingParameters:  routingParameters,
		lifetime:           lifetime,
	}
}

func (s *adminServiceProxyServer) AddOrUpdateRemoteCluster(ctx context.Context, in0 *adminservice.AddOrUpdateRemoteClusterRequest) (resp *adminservice.AddOrUpdateRemoteClusterResponse, err error) {
	s.loggers.Get(logging.AdminService).Info("Received AddOrUpdateRemoteCluster", tag.Address(in0.FrontendAddress), tag.NewBoolTag("Enabled", in0.GetEnableRemoteClusterConnection()), tag.NewStringsTag("configTags", s.metricLabelValues))
	if !common.IsRequestTranslationDisabled(ctx) && len(s.overrides.ReplicationEndpoint) > 0 {
		// Override this address so that cross-cluster connections flow through the proxy.
		// Use a separate "external address" config option because the outbound.listenerAddress may not be routable
		// from the local temporal server, or the proxy may be deployed behind a load balancer.
		// Only used in single-proxy scenarios, i.e. Temporal <> Proxy <> Temporal
		in0.FrontendAddress = s.overrides.ReplicationEndpoint
		s.loggers.Get(logging.AdminService).Info("Overwrote outbound address", tag.Address(in0.FrontendAddress), tag.NewStringsTag("configTags", s.metricLabelValues))
	}
	resp, err = s.adminClient.AddOrUpdateRemoteCluster(ctx, in0)
	if err != nil {
		s.loggers.Get(logging.AdminService).Error("Error when adding remote cluster", tag.Error(err), tag.Operation("AddOrUpdateRemoteCluster"),
			tag.NewStringTag("FrontendAddress", in0.GetFrontendAddress()))
	}
	return
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
	s.loggers.Get(logging.AdminService).Info("Received DescribeClusterRequest")
	resp, err := s.adminClient.DescribeCluster(ctx, in0)

	// If err != nil, resp is always nil, but the linter doesn't know that. Check both so that we can skip all the warnings.
	if err != nil || resp == nil {
		s.loggers.Get(logging.AdminService).Info("Got error when calling DescribeCluster!", tag.Error(err), tag.NewStringsTag("configTags", s.metricLabelValues))
		return resp, err
	}
	s.loggers.Get(logging.AdminService).Info("Raw DescribeClusterResponse", tag.NewStringTag("clusterID", resp.ClusterId),
		tag.NewStringTag("clusterName", resp.ClusterName), tag.NewStringTag("version", resp.ServerVersion),
		tag.NewInt64("failoverVersionIncrement", resp.FailoverVersionIncrement), tag.NewInt64("initialFailoverVersion", resp.InitialFailoverVersion),
		tag.NewBoolTag("isGlobalNamespaceEnabled", resp.IsGlobalNamespaceEnabled), tag.NewStringsTag("configTags", s.metricLabelValues))

	if common.IsRequestTranslationDisabled(ctx) {
		s.loggers.Get(logging.AdminService).Info("Request translation disabled. Returning as-is")
		return resp, err
	}

	switch s.shardCountConfig.Mode {
	case config.ShardCountLCM:
		// Present a fake number of shards. In LCM mode, we present the least
		// common multiple of both cluster shard counts.
		resp.HistoryShardCount = s.lcmParameters.LCM
	case config.ShardCountRouting:
		if s.routingParameters.OverrideShardCount > 0 {
			resp.HistoryShardCount = s.routingParameters.OverrideShardCount
		}
	}

	if s.overrides.FVI != 0 {
		resp.FailoverVersionIncrement = s.overrides.FVI
		s.loggers.Get(logging.AdminService).Info("Overwrite FailoverVersionIncrement", tag.NewInt64("failoverVersionIncrement", resp.FailoverVersionIncrement),
			tag.NewStringsTag("configTags", s.metricLabelValues))
	}

	s.loggers.Get(logging.AdminService).Info("Sending translated DescribeClusterResponse", tag.NewStringTag("clusterID", resp.ClusterId),
		tag.NewStringTag("clusterName", resp.ClusterName), tag.NewStringTag("version", resp.ServerVersion),
		tag.NewInt64("failoverVersionIncrement", resp.FailoverVersionIncrement), tag.NewInt64("initialFailoverVersion", resp.InitialFailoverVersion),
		tag.NewBoolTag("isGlobalNamespaceEnabled", resp.IsGlobalNamespaceEnabled), tag.NewStringsTag("configTags", s.metricLabelValues))
	return resp, err
}

func (s *adminServiceProxyServer) DescribeDLQJob(ctx context.Context, in0 *adminservice.DescribeDLQJobRequest) (*adminservice.DescribeDLQJobResponse, error) {
	return s.adminClient.DescribeDLQJob(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeHistoryHost(ctx context.Context, in0 *adminservice.DescribeHistoryHostRequest) (*adminservice.DescribeHistoryHostResponse, error) {
	return s.adminClient.DescribeHistoryHost(ctx, in0)
}

func (s *adminServiceProxyServer) DescribeMutableState(ctx context.Context, in0 *adminservice.DescribeMutableStateRequest) (resp *adminservice.DescribeMutableStateResponse, err error) {
	resp, err = s.adminClient.DescribeMutableState(ctx, in0)
	if err != nil {
		// This is a duplicate of the grpc client metrics, but not everyone has metrics set up
		s.loggers.Get(logging.ReplicationStreams).Error("Failed to describe workflow",
			tag.NewStringTag("WorkflowId", in0.GetExecution().GetWorkflowId()),
			tag.NewStringTag("RunId", in0.GetExecution().GetRunId()),
			tag.NewStringTag("Namespace", in0.GetNamespace()),
			tag.Error(err), tag.Operation("DescribeMutableState"))
	}
	return
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

func (s *adminServiceProxyServer) GetNamespaceReplicationMessages(ctx context.Context, in0 *adminservice.GetNamespaceReplicationMessagesRequest) (resp *adminservice.GetNamespaceReplicationMessagesResponse, err error) {
	resp, err = s.adminClient.GetNamespaceReplicationMessages(ctx, in0)
	if err != nil {
		// This is a duplicate of the grpc client metrics, but not everyone has metrics set up
		s.loggers.Get(logging.ReplicationStreams).Error("Failed to get namespace replication messages", tag.NewStringTag("Cluster", in0.GetClusterName()),
			tag.Error(err), tag.Operation("GetNamespaceReplicationMessages"))
	}
	return
}

func (s *adminServiceProxyServer) GetReplicationMessages(ctx context.Context, in0 *adminservice.GetReplicationMessagesRequest) (resp *adminservice.GetReplicationMessagesResponse, err error) {
	resp, err = s.adminClient.GetReplicationMessages(ctx, in0)
	if err != nil {
		s.loggers.Get(logging.ReplicationStreams).Error("Failed to get replication messages", tag.NewStringTag("Cluster", in0.GetClusterName()),
			tag.Error(err), tag.Operation("GetReplicationMessages"))
	}
	return
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

func ClusterShardIDtoShortString(sd history.ClusterShardID) string {
	return fmt.Sprintf("%d:%d", sd.ClusterID, sd.ShardID)
}

// StreamWorkflowReplicationMessages establishes an HTTP/2 stream. gRPC passes us a stream that represents the initiating server,
// and we can freely Send and Recv on that "server". Because this is a proxy, we also establish a bidirectional
// stream using our configured adminClient. When we Recv on the initiator, we Send to the client.
// When we Recv on the client, we Send to the initiator
func (s *adminServiceProxyServer) StreamWorkflowReplicationMessages(
	streamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
) (retError error) {
	defer log.CapturePanic(s.loggers.Get(logging.ReplicationStreams), &retError)

	targetMetadata, ok := metadata.FromIncomingContext(streamServer.Context())
	if !ok {
		return serviceerror.NewInvalidArgument("missing cluster & shard ID metadata")
	}
	targetClusterShardID, sourceClusterShardID, err := history.DecodeClusterShardMD(
		headers.NewGRPCHeaderGetter(streamServer.Context()),
	)
	if err != nil {
		return err
	}

	logger := log.With(s.loggers.Get(logging.ReplicationStreams),
		tag.NewStringTag("source", ClusterShardIDtoString(sourceClusterShardID)),
		tag.NewStringTag("target", ClusterShardIDtoString(targetClusterShardID)))

	// Record streams active: the observer will report which exact streams are active, the gauge tells us the count
	s.reportStreamValue(sourceClusterShardID.ShardID, 1)
	defer s.reportStreamValue(sourceClusterShardID.ShardID, -1)
	streamsActiveGauge := metrics.AdminServiceStreamsActive.WithLabelValues(s.metricLabelValues...)
	streamsActiveGauge.Inc()
	defer streamsActiveGauge.Dec()

	err = handleStream(
		streamServer,
		targetMetadata,
		sourceClusterShardID,
		targetClusterShardID,
		logger,
		s.shardCountConfig,
		s.lcmParameters,
		s.routingParameters,
		s.adminClient,
		s.adminClientReverse,
		s.shardManager,
		s.metricLabelValues,
		s.lifetime,
	)
	if err != nil {
		return err
	}
	// Do not try to transfer EOF from the source here. Just returning "nil" is sufficient to terminate the stream
	// to the client.
	return nil
}
