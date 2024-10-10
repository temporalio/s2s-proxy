package proxy

import (
	"context"
	"fmt"
	"io"
	"sync"

	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"

	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

type (
	echoAdminService struct {
		adminservice.UnimplementedAdminServiceServer
		serviceName string
		logger      log.Logger
	}
)

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
	return &adminservice.DescribeClusterResponse{
		ClusterName: s.serviceName,
	}, nil
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
