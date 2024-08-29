package proxy

import (
	"context"
	"fmt"
	"io"
	"sync"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

type (
	mockConfigProvider struct {
		proxyConfig config.S2SProxyConfig
	}

	clusterInfo struct {
		serverAddress  string
		clusterShardID history.ClusterShardID
		s2sProxyConfig *config.S2SProxyConfig // if provided, used for setting up proxy
	}

	echoAdminService struct {
		adminservice.UnimplementedAdminServiceServer
		serviceName string
		logger      log.Logger
	}

	echoWorkflowService struct {
		workflowservice.UnimplementedWorkflowServiceServer
		serviceName string
		logger      log.Logger
	}

	echoServer struct {
		server      *s2sproxy.TemporalAPIServer
		proxy       *s2sproxy.Proxy
		clusterInfo clusterInfo
	}
)

func (mc *mockConfigProvider) GetS2SProxyConfig() config.S2SProxyConfig {
	return mc.proxyConfig
}

// Echo server for testing stream replication. It acts as stream sender.
// It handles StreamWorkflowReplicationMessages call from Echo client (acts as stream receiver) by
// echoing back InclusiveLowWatermark in SyncReplicationState message.
func newEchoServer(
	localClusterInfo clusterInfo,
	remoteClusterInfo clusterInfo,
	logger log.Logger,
) *echoServer {
	senderAdminService := &echoAdminService{
		serviceName: "EchoServer",
		logger:      log.With(logger, common.ServiceTag("EchoServer"), tag.Address(localClusterInfo.serverAddress)),
	}
	senderWorkflowService := &echoWorkflowService{
		serviceName: "EchoServer",
		logger:      log.With(logger, common.ServiceTag("EchoServer"), tag.Address(localClusterInfo.serverAddress)),
	}

	var proxy *s2sproxy.Proxy
	var err error

	if localCfg := localClusterInfo.s2sProxyConfig; localCfg != nil {
		if remoteCfg := remoteClusterInfo.s2sProxyConfig; remoteCfg != nil {
			localCfg.Outbound.Client.ForwardAddress = remoteCfg.Inbound.Server.ListenAddress
		} else {
			localCfg.Outbound.Client.ForwardAddress = remoteClusterInfo.serverAddress
		}

		configProvider := &mockConfigProvider{
			proxyConfig: *localClusterInfo.s2sProxyConfig,
		}

		rpcFactory := rpc.NewRPCFactory(configProvider, logger)
		clientFactory := client.NewClientFactory(rpcFactory, logger)
		proxy, err = s2sproxy.NewProxy(
			configProvider,
			logger,
			clientFactory,
		)

		if err != nil {
			logger.Fatal("Failed to create proxy", tag.Error(err))
		}
	}

	return &echoServer{
		server: s2sproxy.NewTemporalAPIServer(
			"EchoServer",
			config.ServerConfig{
				ListenAddress: localClusterInfo.serverAddress,
			},
			senderAdminService,
			senderWorkflowService,
			nil,
			logger),
		proxy:       proxy,
		clusterInfo: localClusterInfo,
	}
}

func (s *echoServer) start() {
	_ = s.server.Start()
	if s.proxy != nil {
		_ = s.proxy.Start()
	}
}

func (s *echoServer) stop() {
	if s.proxy != nil {
		s.proxy.Stop()
	}
	s.server.Stop()
}

func (s *echoAdminService) AddOrUpdateRemoteCluster(ctx context.Context, in0 *adminservice.AddOrUpdateRemoteClusterRequest) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddOrUpdateRemoteCluster is not allowed.")
}

func (s *echoAdminService) AddSearchAttributes(ctx context.Context, in0 *adminservice.AddSearchAttributesRequest) (*adminservice.AddSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddSearchAttributes is not allowed.")
}

func (s *echoAdminService) AddTasks(ctx context.Context, in0 *adminservice.AddTasksRequest) (*adminservice.AddTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddTasks is not allowed.")
}

func (s *echoAdminService) CancelDLQJob(ctx context.Context, in0 *adminservice.CancelDLQJobRequest) (*adminservice.CancelDLQJobResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method CancelDLQJob is not allowed.")
}

func (s *echoAdminService) CloseShard(ctx context.Context, in0 *adminservice.CloseShardRequest) (*adminservice.CloseShardResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method CloseShard is not allowed.")
}

func (s *echoAdminService) DeleteWorkflowExecution(ctx context.Context, in0 *adminservice.DeleteWorkflowExecutionRequest) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *echoAdminService) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *echoAdminService) DescribeDLQJob(ctx context.Context, in0 *adminservice.DescribeDLQJobRequest) (*adminservice.DescribeDLQJobResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeDLQJob is not allowed.")
}

func (s *echoAdminService) DescribeHistoryHost(ctx context.Context, in0 *adminservice.DescribeHistoryHostRequest) (*adminservice.DescribeHistoryHostResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeHistoryHost is not allowed.")
}

func (s *echoAdminService) DescribeMutableState(ctx context.Context, in0 *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeMutableState is not allowed.")
}

func (s *echoAdminService) GetDLQMessages(ctx context.Context, in0 *adminservice.GetDLQMessagesRequest) (*adminservice.GetDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQMessages is not allowed.")
}

func (s *echoAdminService) GetDLQReplicationMessages(ctx context.Context, in0 *adminservice.GetDLQReplicationMessagesRequest) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQReplicationMessages is not allowed.")
}

func (s *echoAdminService) GetDLQTasks(ctx context.Context, in0 *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQTasks is not allowed.")
}

func (s *echoAdminService) GetNamespace(ctx context.Context, in0 *adminservice.GetNamespaceRequest) (*adminservice.GetNamespaceResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetNamespace is not allowed.")
}

func (s *echoAdminService) GetNamespaceReplicationMessages(ctx context.Context, in0 *adminservice.GetNamespaceReplicationMessagesRequest) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *echoAdminService) GetReplicationMessages(ctx context.Context, in0 *adminservice.GetReplicationMessagesRequest) (*adminservice.GetReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetReplicationMessages is not allowed.")
}

func (s *echoAdminService) GetSearchAttributes(ctx context.Context, in0 *adminservice.GetSearchAttributesRequest) (*adminservice.GetSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetSearchAttributes is not allowed.")
}

func (s *echoAdminService) GetShard(ctx context.Context, in0 *adminservice.GetShardRequest) (*adminservice.GetShardResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetShard is not allowed.")
}

func (s *echoAdminService) GetTaskQueueTasks(ctx context.Context, in0 *adminservice.GetTaskQueueTasksRequest) (*adminservice.GetTaskQueueTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetTaskQueueTasks is not allowed.")
}

func (s *echoAdminService) GetWorkflowExecutionRawHistory(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryRequest) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionRawHistory is not allowed.")
}

func (s *echoAdminService) GetWorkflowExecutionRawHistoryV2(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryV2Request) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *echoAdminService) ImportWorkflowExecution(ctx context.Context, in0 *adminservice.ImportWorkflowExecutionRequest) (*adminservice.ImportWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ImportWorkflowExecution is not allowed.")
}

func (s *echoAdminService) ListClusterMembers(ctx context.Context, in0 *adminservice.ListClusterMembersRequest) (*adminservice.ListClusterMembersResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusterMembers is not allowed.")
}

func (s *echoAdminService) ListClusters(ctx context.Context, in0 *adminservice.ListClustersRequest) (*adminservice.ListClustersResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusterMembers is not allowed.")
}

func (s *echoAdminService) ListHistoryTasks(ctx context.Context, in0 *adminservice.ListHistoryTasksRequest) (*adminservice.ListHistoryTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListHistoryTasks is not allowed.")
}

func (s *echoAdminService) ListQueues(ctx context.Context, in0 *adminservice.ListQueuesRequest) (*adminservice.ListQueuesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListQueues is not allowed.")
}

func (s *echoAdminService) MergeDLQMessages(ctx context.Context, in0 *adminservice.MergeDLQMessagesRequest) (*adminservice.MergeDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQMessages is not allowed.")
}

func (s *echoAdminService) MergeDLQTasks(ctx context.Context, in0 *adminservice.MergeDLQTasksRequest) (*adminservice.MergeDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQTasks is not allowed.")
}

func (s *echoAdminService) PurgeDLQMessages(ctx context.Context, in0 *adminservice.PurgeDLQMessagesRequest) (*adminservice.PurgeDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQMessages is not allowed.")
}

func (s *echoAdminService) PurgeDLQTasks(ctx context.Context, in0 *adminservice.PurgeDLQTasksRequest) (*adminservice.PurgeDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQTasks is not allowed.")
}

func (s *echoAdminService) ReapplyEvents(ctx context.Context, in0 *adminservice.ReapplyEventsRequest) (*adminservice.ReapplyEventsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ReapplyEvents is not allowed.")
}

func (s *echoAdminService) RebuildMutableState(ctx context.Context, in0 *adminservice.RebuildMutableStateRequest) (*adminservice.RebuildMutableStateResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RebuildMutableState is not allowed.")
}

func (s *echoAdminService) RefreshWorkflowTasks(ctx context.Context, in0 *adminservice.RefreshWorkflowTasksRequest) (*adminservice.RefreshWorkflowTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RefreshWorkflowTasks is not allowed.")
}

func (s *echoAdminService) RemoveRemoteCluster(ctx context.Context, in0 *adminservice.RemoveRemoteClusterRequest) (*adminservice.RemoveRemoteClusterResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveRemoteCluster is not allowed.")
}

func (s *echoAdminService) RemoveSearchAttributes(ctx context.Context, in0 *adminservice.RemoveSearchAttributesRequest) (*adminservice.RemoveSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveSearchAttributes is not allowed.")
}

func (s *echoAdminService) RemoveTask(ctx context.Context, in0 *adminservice.RemoveTaskRequest) (*adminservice.RemoveTaskResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveTask is not allowed.")
}

func (s *echoAdminService) ResendReplicationTasks(ctx context.Context, in0 *adminservice.ResendReplicationTasksRequest) (*adminservice.ResendReplicationTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ResendReplicationTasks is not allowed.")
}

func (s *echoAdminService) StreamWorkflowReplicationMessages(
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
) (retError error) {
	defer log.CapturePanic(s.logger, &retError)

	targetMetadata, ok := metadata.FromIncomingContext(targetStreamServer.Context())
	if !ok {
		return serviceerror.NewInvalidArgument("missing cluster & shard ID metadata")
	}
	targetClusterShardID, sourceClusterShardID, err := history.DecodeClusterShardMD(targetMetadata)
	if err != nil {
		return err
	}

	logger := log.With(s.logger,
		tag.NewStringTag("target", s2sproxy.ClusterShardIDtoString(targetClusterShardID)),
		tag.NewStringTag("source", s2sproxy.ClusterShardIDtoString(sourceClusterShardID)))

	logger.Info("AdminStreamReplicationMessages started.")
	defer logger.Info("AdminStreamReplicationMessages stopped.")

	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()

		for {
			req, err := targetStreamServer.Recv()
			if err == io.EOF {
				return
			}

			if err != nil {
				logger.Error("targetStreamServer.Recv encountered error", tag.Error(err))
				retError = err
				return
			}

			switch attr := req.GetAttributes().(type) {
			case *adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState:
				req := &adminservice.StreamWorkflowReplicationMessagesResponse{
					Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
						Messages: &replicationpb.WorkflowReplicationMessages{
							ExclusiveHighWatermark: attr.SyncReplicationState.HighPriorityState.InclusiveLowWatermark,
						},
					}}

				if err = targetStreamServer.Send(req); err != nil {
					logger.Error("targetStreamServer.Send encountered error", tag.Error(err))
					retError = err
					return
				}

			default:
				logger.Error("targetStreamServer.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
					"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
				))))
				retError = fmt.Errorf("targetStreamServer.Recv unknown type")
				return
			}
		}
	}()

	wg.Wait()
	return
}

func (s *echoWorkflowService) CountWorkflowExecutions(ctx context.Context, in0 *workflowservice.CountWorkflowExecutionsRequest) (*workflowservice.CountWorkflowExecutionsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method CountWorkflowExecutions is not allowed.")
}

func (s *echoWorkflowService) CreateSchedule(ctx context.Context, in0 *workflowservice.CreateScheduleRequest) (*workflowservice.CreateScheduleResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method CreateSchedule is not allowed.")
}

func (s *echoWorkflowService) DeleteSchedule(ctx context.Context, in0 *workflowservice.DeleteScheduleRequest) (*workflowservice.DeleteScheduleResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteSchedule is not allowed.")
}

func (s *echoWorkflowService) DeleteWorkflowExecution(ctx context.Context, in0 *workflowservice.DeleteWorkflowExecutionRequest) (*workflowservice.DeleteWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *echoWorkflowService) DeprecateNamespace(ctx context.Context, in0 *workflowservice.DeprecateNamespaceRequest) (*workflowservice.DeprecateNamespaceResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeprecateNamespace is not allowed.")
}

func (s *echoWorkflowService) DescribeBatchOperation(ctx context.Context, in0 *workflowservice.DescribeBatchOperationRequest) (*workflowservice.DescribeBatchOperationResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeBatchOperation is not allowed.")
}

func (s *echoWorkflowService) DescribeNamespace(ctx context.Context, in0 *workflowservice.DescribeNamespaceRequest) (*workflowservice.DescribeNamespaceResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeNamespace is not allowed.")
}

func (s *echoWorkflowService) DescribeSchedule(ctx context.Context, in0 *workflowservice.DescribeScheduleRequest) (*workflowservice.DescribeScheduleResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeSchedule is not allowed.")
}

func (s *echoWorkflowService) DescribeTaskQueue(ctx context.Context, in0 *workflowservice.DescribeTaskQueueRequest) (*workflowservice.DescribeTaskQueueResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeTaskQueue is not allowed.")
}

func (s *echoWorkflowService) DescribeWorkflowExecution(ctx context.Context, in0 *workflowservice.DescribeWorkflowExecutionRequest) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeWorkflowExecution is not allowed.")
}

func (s *echoWorkflowService) ExecuteMultiOperation(ctx context.Context, in0 *workflowservice.ExecuteMultiOperationRequest) (*workflowservice.ExecuteMultiOperationResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ExecuteMultiOperation is not allowed.")
}

func (s *echoWorkflowService) GetClusterInfo(ctx context.Context, in0 *workflowservice.GetClusterInfoRequest) (*workflowservice.GetClusterInfoResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetClusterInfo is not allowed.")
}

func (s *echoWorkflowService) GetSearchAttributes(ctx context.Context, in0 *workflowservice.GetSearchAttributesRequest) (*workflowservice.GetSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetSearchAttributes is not allowed.")
}

func (s *echoWorkflowService) GetSystemInfo(ctx context.Context, in0 *workflowservice.GetSystemInfoRequest) (*workflowservice.GetSystemInfoResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetSystemInfo is not allowed.")
}

func (s *echoWorkflowService) GetWorkerBuildIdCompatibility(ctx context.Context, in0 *workflowservice.GetWorkerBuildIdCompatibilityRequest) (*workflowservice.GetWorkerBuildIdCompatibilityResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkerBuildIdCompatibility is not allowed.")
}

func (s *echoWorkflowService) GetWorkerTaskReachability(ctx context.Context, in0 *workflowservice.GetWorkerTaskReachabilityRequest) (*workflowservice.GetWorkerTaskReachabilityResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkerTaskReachability is not allowed.")
}

func (s *echoWorkflowService) GetWorkerVersioningRules(ctx context.Context, in0 *workflowservice.GetWorkerVersioningRulesRequest) (*workflowservice.GetWorkerVersioningRulesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkerVersioningRules is not allowed.")
}

func (s *echoWorkflowService) GetWorkflowExecutionHistory(ctx context.Context, in0 *workflowservice.GetWorkflowExecutionHistoryRequest) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionHistory is not allowed.")
}

func (s *echoWorkflowService) GetWorkflowExecutionHistoryReverse(ctx context.Context, in0 *workflowservice.GetWorkflowExecutionHistoryReverseRequest) (*workflowservice.GetWorkflowExecutionHistoryReverseResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionHistoryReverse is not allowed.")
}

func (s *echoWorkflowService) ListArchivedWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListArchivedWorkflowExecutionsRequest) (*workflowservice.ListArchivedWorkflowExecutionsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListArchivedWorkflowExecutions is not allowed.")
}

func (s *echoWorkflowService) ListBatchOperations(ctx context.Context, in0 *workflowservice.ListBatchOperationsRequest) (*workflowservice.ListBatchOperationsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListBatchOperations is not allowed.")
}

func (s *echoWorkflowService) ListClosedWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListClosedWorkflowExecutionsRequest) (*workflowservice.ListClosedWorkflowExecutionsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClosedWorkflowExecutions is not allowed.")
}

func (s *echoWorkflowService) ListNamespaces(ctx context.Context, in0 *workflowservice.ListNamespacesRequest) (*workflowservice.ListNamespacesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListNamespaces is not allowed.")
}

func (s *echoWorkflowService) ListOpenWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListOpenWorkflowExecutionsRequest) (*workflowservice.ListOpenWorkflowExecutionsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListOpenWorkflowExecutions is not allowed.")
}

func (s *echoWorkflowService) ListScheduleMatchingTimes(ctx context.Context, in0 *workflowservice.ListScheduleMatchingTimesRequest) (*workflowservice.ListScheduleMatchingTimesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListScheduleMatchingTimes is not allowed.")
}

func (s *echoWorkflowService) ListSchedules(ctx context.Context, in0 *workflowservice.ListSchedulesRequest) (*workflowservice.ListSchedulesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListSchedules is not allowed.")
}

func (s *echoWorkflowService) ListTaskQueuePartitions(ctx context.Context, in0 *workflowservice.ListTaskQueuePartitionsRequest) (*workflowservice.ListTaskQueuePartitionsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListTaskQueuePartitions is not allowed.")
}

func (s *echoWorkflowService) ListWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListWorkflowExecutionsRequest) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListWorkflowExecutions is not allowed.")
}

func (s *echoWorkflowService) PatchSchedule(ctx context.Context, in0 *workflowservice.PatchScheduleRequest) (*workflowservice.PatchScheduleResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PatchSchedule is not allowed.")
}

func (s *echoWorkflowService) PollActivityTaskQueue(ctx context.Context, in0 *workflowservice.PollActivityTaskQueueRequest) (*workflowservice.PollActivityTaskQueueResponse, error) {
	resp := &workflowservice.PollActivityTaskQueueResponse{
		WorkflowNamespace: in0.Namespace,
	}
	s.logger.Info("PollActivityTaskQueue", tag.NewAnyTag("req", in0), tag.NewAnyTag("resp", resp))
	return resp, nil
}

func (s *echoWorkflowService) PollNexusTaskQueue(ctx context.Context, in0 *workflowservice.PollNexusTaskQueueRequest) (*workflowservice.PollNexusTaskQueueResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PollNexusTaskQueue is not allowed.")
}

func (s *echoWorkflowService) PollWorkflowExecutionUpdate(ctx context.Context, in0 *workflowservice.PollWorkflowExecutionUpdateRequest) (*workflowservice.PollWorkflowExecutionUpdateResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PollWorkflowExecutionUpdate is not allowed.")
}

func (s *echoWorkflowService) PollWorkflowTaskQueue(ctx context.Context, in0 *workflowservice.PollWorkflowTaskQueueRequest) (*workflowservice.PollWorkflowTaskQueueResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PollWorkflowTaskQueue is not allowed.")
}

func (s *echoWorkflowService) QueryWorkflow(ctx context.Context, in0 *workflowservice.QueryWorkflowRequest) (*workflowservice.QueryWorkflowResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method QueryWorkflow is not allowed.")
}

func (s *echoWorkflowService) RecordActivityTaskHeartbeat(ctx context.Context, in0 *workflowservice.RecordActivityTaskHeartbeatRequest) (*workflowservice.RecordActivityTaskHeartbeatResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RecordActivityTaskHeartbeat is not allowed.")
}

func (s *echoWorkflowService) RecordActivityTaskHeartbeatById(ctx context.Context, in0 *workflowservice.RecordActivityTaskHeartbeatByIdRequest) (*workflowservice.RecordActivityTaskHeartbeatByIdResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RecordActivityTaskHeartbeatById is not allowed.")
}

func (s *echoWorkflowService) RegisterNamespace(ctx context.Context, in0 *workflowservice.RegisterNamespaceRequest) (*workflowservice.RegisterNamespaceResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RegisterNamespace is not allowed.")
}

func (s *echoWorkflowService) RequestCancelWorkflowExecution(ctx context.Context, in0 *workflowservice.RequestCancelWorkflowExecutionRequest) (*workflowservice.RequestCancelWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RequestCancelWorkflowExecution is not allowed.")
}

func (s *echoWorkflowService) ResetStickyTaskQueue(ctx context.Context, in0 *workflowservice.ResetStickyTaskQueueRequest) (*workflowservice.ResetStickyTaskQueueResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ResetStickyTaskQueue is not allowed.")
}

func (s *echoWorkflowService) ResetWorkflowExecution(ctx context.Context, in0 *workflowservice.ResetWorkflowExecutionRequest) (*workflowservice.ResetWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ResetWorkflowExecution is not allowed.")
}

func (s *echoWorkflowService) RespondActivityTaskCanceled(ctx context.Context, in0 *workflowservice.RespondActivityTaskCanceledRequest) (*workflowservice.RespondActivityTaskCanceledResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondActivityTaskCanceled is not allowed.")
}

func (s *echoWorkflowService) RespondActivityTaskCanceledById(ctx context.Context, in0 *workflowservice.RespondActivityTaskCanceledByIdRequest) (*workflowservice.RespondActivityTaskCanceledByIdResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondActivityTaskCanceledById is not allowed.")
}

func (s *echoWorkflowService) RespondActivityTaskCompleted(ctx context.Context, in0 *workflowservice.RespondActivityTaskCompletedRequest) (*workflowservice.RespondActivityTaskCompletedResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondActivityTaskCompleted is not allowed.")
}

func (s *echoWorkflowService) RespondActivityTaskCompletedById(ctx context.Context, in0 *workflowservice.RespondActivityTaskCompletedByIdRequest) (*workflowservice.RespondActivityTaskCompletedByIdResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondActivityTaskCompletedById is not allowed.")
}

func (s *echoWorkflowService) RespondActivityTaskFailed(ctx context.Context, in0 *workflowservice.RespondActivityTaskFailedRequest) (*workflowservice.RespondActivityTaskFailedResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondActivityTaskFailed is not allowed.")
}

func (s *echoWorkflowService) RespondActivityTaskFailedById(ctx context.Context, in0 *workflowservice.RespondActivityTaskFailedByIdRequest) (*workflowservice.RespondActivityTaskFailedByIdResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondActivityTaskFailedById is not allowed.")
}

func (s *echoWorkflowService) RespondNexusTaskCompleted(ctx context.Context, in0 *workflowservice.RespondNexusTaskCompletedRequest) (*workflowservice.RespondNexusTaskCompletedResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondNexusTaskCompleted is not allowed.")
}

func (s *echoWorkflowService) RespondNexusTaskFailed(ctx context.Context, in0 *workflowservice.RespondNexusTaskFailedRequest) (*workflowservice.RespondNexusTaskFailedResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondNexusTaskFailed is not allowed.")
}

func (s *echoWorkflowService) RespondQueryTaskCompleted(ctx context.Context, in0 *workflowservice.RespondQueryTaskCompletedRequest) (*workflowservice.RespondQueryTaskCompletedResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondQueryTaskCompleted is not allowed.")
}

func (s *echoWorkflowService) RespondWorkflowTaskCompleted(ctx context.Context, in0 *workflowservice.RespondWorkflowTaskCompletedRequest) (*workflowservice.RespondWorkflowTaskCompletedResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondWorkflowTaskCompleted is not allowed.")
}

func (s *echoWorkflowService) RespondWorkflowTaskFailed(ctx context.Context, in0 *workflowservice.RespondWorkflowTaskFailedRequest) (*workflowservice.RespondWorkflowTaskFailedResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RespondWorkflowTaskFailed is not allowed.")
}

func (s *echoWorkflowService) ScanWorkflowExecutions(ctx context.Context, in0 *workflowservice.ScanWorkflowExecutionsRequest) (*workflowservice.ScanWorkflowExecutionsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ScanWorkflowExecutions is not allowed.")
}

func (s *echoWorkflowService) SignalWithStartWorkflowExecution(ctx context.Context, in0 *workflowservice.SignalWithStartWorkflowExecutionRequest) (*workflowservice.SignalWithStartWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method SignalWithStartWorkflowExecution is not allowed.")
}

func (s *echoWorkflowService) SignalWorkflowExecution(ctx context.Context, in0 *workflowservice.SignalWorkflowExecutionRequest) (*workflowservice.SignalWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method SignalWorkflowExecution is not allowed.")
}

func (s *echoWorkflowService) StartBatchOperation(ctx context.Context, in0 *workflowservice.StartBatchOperationRequest) (*workflowservice.StartBatchOperationResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method StartBatchOperation is not allowed.")
}

func (s *echoWorkflowService) StartWorkflowExecution(ctx context.Context, in0 *workflowservice.StartWorkflowExecutionRequest) (*workflowservice.StartWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method StartWorkflowExecution is not allowed.")
}

func (s *echoWorkflowService) StopBatchOperation(ctx context.Context, in0 *workflowservice.StopBatchOperationRequest) (*workflowservice.StopBatchOperationResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method StopBatchOperation is not allowed.")
}

func (s *echoWorkflowService) TerminateWorkflowExecution(ctx context.Context, in0 *workflowservice.TerminateWorkflowExecutionRequest) (*workflowservice.TerminateWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method TerminateWorkflowExecution is not allowed.")
}

func (s *echoWorkflowService) UpdateNamespace(ctx context.Context, in0 *workflowservice.UpdateNamespaceRequest) (*workflowservice.UpdateNamespaceResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method UpdateNamespace is not allowed.")
}

func (s *echoWorkflowService) UpdateSchedule(ctx context.Context, in0 *workflowservice.UpdateScheduleRequest) (*workflowservice.UpdateScheduleResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method UpdateSchedule is not allowed.")
}

func (s *echoWorkflowService) UpdateWorkerBuildIdCompatibility(ctx context.Context, in0 *workflowservice.UpdateWorkerBuildIdCompatibilityRequest) (*workflowservice.UpdateWorkerBuildIdCompatibilityResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method UpdateWorkerBuildIdCompatibility is not allowed.")
}

func (s *echoWorkflowService) UpdateWorkerVersioningRules(ctx context.Context, in0 *workflowservice.UpdateWorkerVersioningRulesRequest) (*workflowservice.UpdateWorkerVersioningRulesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method UpdateWorkerVersioningRules is not allowed.")
}

func (s *echoWorkflowService) UpdateWorkflowExecution(ctx context.Context, in0 *workflowservice.UpdateWorkflowExecutionRequest) (*workflowservice.UpdateWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method UpdateWorkflowExecution is not allowed.")
}
