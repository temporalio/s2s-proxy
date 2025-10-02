package services

import (
	"context"
	"fmt"
	"io"
	"sync"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/headers"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type (
	EchoAdminService struct {
		adminservice.UnimplementedAdminServiceServer
		ServiceName string
		Namespaces  map[string]bool
		Logger      log.Logger
		PayloadSize int
	}
)

func (s *EchoAdminService) AddOrUpdateRemoteCluster(ctx context.Context, in0 *adminservice.AddOrUpdateRemoteClusterRequest) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddOrUpdateRemoteCluster is not allowed.")
}

func (s *EchoAdminService) AddSearchAttributes(ctx context.Context, in0 *adminservice.AddSearchAttributesRequest) (*adminservice.AddSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddSearchAttributes is not allowed.")
}

func (s *EchoAdminService) AddTasks(ctx context.Context, in0 *adminservice.AddTasksRequest) (*adminservice.AddTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddTasks is not allowed.")
}

func (s *EchoAdminService) CancelDLQJob(ctx context.Context, in0 *adminservice.CancelDLQJobRequest) (*adminservice.CancelDLQJobResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method CancelDLQJob is not allowed.")
}

func (s *EchoAdminService) CloseShard(ctx context.Context, in0 *adminservice.CloseShardRequest) (*adminservice.CloseShardResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method CloseShard is not allowed.")
}

func (s *EchoAdminService) DeleteWorkflowExecution(ctx context.Context, in0 *adminservice.DeleteWorkflowExecutionRequest) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *EchoAdminService) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	return &adminservice.DescribeClusterResponse{
		ClusterName: s.ServiceName,
	}, nil
}

func (s *EchoAdminService) DescribeDLQJob(ctx context.Context, in0 *adminservice.DescribeDLQJobRequest) (*adminservice.DescribeDLQJobResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeDLQJob is not allowed.")
}

func (s *EchoAdminService) DescribeHistoryHost(ctx context.Context, in0 *adminservice.DescribeHistoryHostRequest) (*adminservice.DescribeHistoryHostResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeHistoryHost is not allowed.")
}

func (s *EchoAdminService) DescribeMutableState(ctx context.Context, in0 *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	if !s.Namespaces[in0.Namespace] {
		return nil, status.Errorf(codes.NotFound, "namespace %s is not found", in0.Namespace)
	}

	return &adminservice.DescribeMutableStateResponse{}, nil
}

func (s *EchoAdminService) GetDLQMessages(ctx context.Context, in0 *adminservice.GetDLQMessagesRequest) (*adminservice.GetDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQMessages is not allowed.")
}

func (s *EchoAdminService) GetDLQReplicationMessages(ctx context.Context, in0 *adminservice.GetDLQReplicationMessagesRequest) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQReplicationMessages is not allowed.")
}

func (s *EchoAdminService) GetDLQTasks(ctx context.Context, in0 *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQTasks is not allowed.")
}

func (s *EchoAdminService) GetNamespace(ctx context.Context, in0 *adminservice.GetNamespaceRequest) (*adminservice.GetNamespaceResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetNamespace is not allowed.")
}

func (s *EchoAdminService) GetNamespaceReplicationMessages(ctx context.Context, in0 *adminservice.GetNamespaceReplicationMessagesRequest) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *EchoAdminService) GetReplicationMessages(ctx context.Context, in0 *adminservice.GetReplicationMessagesRequest) (*adminservice.GetReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetReplicationMessages is not allowed.")
}

func (s *EchoAdminService) GetSearchAttributes(ctx context.Context, in0 *adminservice.GetSearchAttributesRequest) (*adminservice.GetSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetSearchAttributes is not allowed.")
}

func (s *EchoAdminService) GetShard(ctx context.Context, in0 *adminservice.GetShardRequest) (*adminservice.GetShardResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetShard is not allowed.")
}

func (s *EchoAdminService) GetTaskQueueTasks(ctx context.Context, in0 *adminservice.GetTaskQueueTasksRequest) (*adminservice.GetTaskQueueTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetTaskQueueTasks is not allowed.")
}

func (s *EchoAdminService) GetWorkflowExecutionRawHistory(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryRequest) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionRawHistory is not allowed.")
}

func (s *EchoAdminService) GetWorkflowExecutionRawHistoryV2(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryV2Request) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *EchoAdminService) ImportWorkflowExecution(ctx context.Context, in0 *adminservice.ImportWorkflowExecutionRequest) (*adminservice.ImportWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ImportWorkflowExecution is not allowed.")
}

func (s *EchoAdminService) ListClusterMembers(ctx context.Context, in0 *adminservice.ListClusterMembersRequest) (*adminservice.ListClusterMembersResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusterMembers is not allowed.")
}

func (s *EchoAdminService) ListClusters(ctx context.Context, in0 *adminservice.ListClustersRequest) (*adminservice.ListClustersResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusterMembers is not allowed.")
}

func (s *EchoAdminService) ListHistoryTasks(ctx context.Context, in0 *adminservice.ListHistoryTasksRequest) (*adminservice.ListHistoryTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListHistoryTasks is not allowed.")
}

func (s *EchoAdminService) ListQueues(ctx context.Context, in0 *adminservice.ListQueuesRequest) (*adminservice.ListQueuesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListQueues is not allowed.")
}

func (s *EchoAdminService) MergeDLQMessages(ctx context.Context, in0 *adminservice.MergeDLQMessagesRequest) (*adminservice.MergeDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQMessages is not allowed.")
}

func (s *EchoAdminService) MergeDLQTasks(ctx context.Context, in0 *adminservice.MergeDLQTasksRequest) (*adminservice.MergeDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQTasks is not allowed.")
}

func (s *EchoAdminService) PurgeDLQMessages(ctx context.Context, in0 *adminservice.PurgeDLQMessagesRequest) (*adminservice.PurgeDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQMessages is not allowed.")
}

func (s *EchoAdminService) PurgeDLQTasks(ctx context.Context, in0 *adminservice.PurgeDLQTasksRequest) (*adminservice.PurgeDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQTasks is not allowed.")
}

func (s *EchoAdminService) ReapplyEvents(ctx context.Context, in0 *adminservice.ReapplyEventsRequest) (*adminservice.ReapplyEventsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ReapplyEvents is not allowed.")
}

func (s *EchoAdminService) RebuildMutableState(ctx context.Context, in0 *adminservice.RebuildMutableStateRequest) (*adminservice.RebuildMutableStateResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RebuildMutableState is not allowed.")
}

func (s *EchoAdminService) RefreshWorkflowTasks(ctx context.Context, in0 *adminservice.RefreshWorkflowTasksRequest) (*adminservice.RefreshWorkflowTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RefreshWorkflowTasks is not allowed.")
}

func (s *EchoAdminService) RemoveRemoteCluster(ctx context.Context, in0 *adminservice.RemoveRemoteClusterRequest) (*adminservice.RemoveRemoteClusterResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveRemoteCluster is not allowed.")
}

func (s *EchoAdminService) RemoveSearchAttributes(ctx context.Context, in0 *adminservice.RemoveSearchAttributesRequest) (*adminservice.RemoveSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveSearchAttributes is not allowed.")
}

func (s *EchoAdminService) RemoveTask(ctx context.Context, in0 *adminservice.RemoveTaskRequest) (*adminservice.RemoveTaskResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveTask is not allowed.")
}

func (s *EchoAdminService) ResendReplicationTasks(ctx context.Context, in0 *adminservice.ResendReplicationTasksRequest) (*adminservice.ResendReplicationTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ResendReplicationTasks is not allowed.")
}

func ClusterShardIDtoString(sd history.ClusterShardID) string {
	return fmt.Sprintf("(id: %d, shard: %d)", sd.ClusterID, sd.ShardID)
}

func (s *EchoAdminService) StreamWorkflowReplicationMessages(
	targetStreamServer adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
) (retError error) {
	defer log.CapturePanic(s.Logger, &retError)

	targetClusterShardID, sourceClusterShardID, err := history.DecodeClusterShardMD(
		headers.NewGRPCHeaderGetter(targetStreamServer.Context()),
	)
	if err != nil {
		return err
	}

	logger := log.With(s.Logger,
		tag.NewStringTag("target", ClusterShardIDtoString(targetClusterShardID)),
		tag.NewStringTag("source", ClusterShardIDtoString(sourceClusterShardID)))

	logger.Info("AdminStreamReplicationMessages started.")
	defer logger.Info("AdminStreamReplicationMessages stopped.")

	var wg sync.WaitGroup
	wg.Add(1)

	buffer := make([]byte, s.PayloadSize)
	message := &replicationpb.WorkflowReplicationMessages{}
	task := replicationpb.ReplicationTask{
		Data: &commonpb.DataBlob{
			Data: buffer,
		},
	}
	message.ReplicationTasks = append(message.ReplicationTasks, &task)

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
				message.ExclusiveHighWatermark = attr.SyncReplicationState.HighPriorityState.InclusiveLowWatermark
				req := &adminservice.StreamWorkflowReplicationMessagesResponse{
					Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
						Messages: message,
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
