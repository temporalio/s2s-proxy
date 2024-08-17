package proxy

import (
	"context"
	"fmt"
	"io"

	"github.com/temporalio/s2s-proxy/common"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	status "google.golang.org/grpc/status"
)

type (
	mockSenderService struct {
		adminservice.UnimplementedAdminServiceServer
		serviceName string
		logger      log.Logger
	}
)

func NewMockSenderService(
	serviceName string,
	logger log.Logger,
) adminservice.AdminServiceServer {
	return &mockSenderService{
		serviceName: serviceName,
		logger:      logger,
	}
}

func (s *mockSenderService) AddOrUpdateRemoteCluster(ctx context.Context, in0 *adminservice.AddOrUpdateRemoteClusterRequest) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddOrUpdateRemoteCluster is not allowed.")
}

func (s *mockSenderService) AddSearchAttributes(ctx context.Context, in0 *adminservice.AddSearchAttributesRequest) (*adminservice.AddSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddSearchAttributes is not allowed.")
}

func (s *mockSenderService) AddTasks(ctx context.Context, in0 *adminservice.AddTasksRequest) (*adminservice.AddTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method AddTasks is not allowed.")
}

func (s *mockSenderService) CancelDLQJob(ctx context.Context, in0 *adminservice.CancelDLQJobRequest) (*adminservice.CancelDLQJobResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method CancelDLQJob is not allowed.")
}

func (s *mockSenderService) CloseShard(ctx context.Context, in0 *adminservice.CloseShardRequest) (*adminservice.CloseShardResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method CloseShard is not allowed.")
}

func (s *mockSenderService) DeleteWorkflowExecution(ctx context.Context, in0 *adminservice.DeleteWorkflowExecutionRequest) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *mockSenderService) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *mockSenderService) DescribeDLQJob(ctx context.Context, in0 *adminservice.DescribeDLQJobRequest) (*adminservice.DescribeDLQJobResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeDLQJob is not allowed.")
}

func (s *mockSenderService) DescribeHistoryHost(ctx context.Context, in0 *adminservice.DescribeHistoryHostRequest) (*adminservice.DescribeHistoryHostResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeHistoryHost is not allowed.")
}

func (s *mockSenderService) DescribeMutableState(ctx context.Context, in0 *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeMutableState is not allowed.")
}

func (s *mockSenderService) GetDLQMessages(ctx context.Context, in0 *adminservice.GetDLQMessagesRequest) (*adminservice.GetDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQMessages is not allowed.")
}

func (s *mockSenderService) GetDLQReplicationMessages(ctx context.Context, in0 *adminservice.GetDLQReplicationMessagesRequest) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQReplicationMessages is not allowed.")
}

func (s *mockSenderService) GetDLQTasks(ctx context.Context, in0 *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQTasks is not allowed.")
}

func (s *mockSenderService) GetNamespace(ctx context.Context, in0 *adminservice.GetNamespaceRequest) (*adminservice.GetNamespaceResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetNamespace is not allowed.")
}

func (s *mockSenderService) GetNamespaceReplicationMessages(ctx context.Context, in0 *adminservice.GetNamespaceReplicationMessagesRequest) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *mockSenderService) GetReplicationMessages(ctx context.Context, in0 *adminservice.GetReplicationMessagesRequest) (*adminservice.GetReplicationMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetReplicationMessages is not allowed.")
}

func (s *mockSenderService) GetSearchAttributes(ctx context.Context, in0 *adminservice.GetSearchAttributesRequest) (*adminservice.GetSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetSearchAttributes is not allowed.")
}

func (s *mockSenderService) GetShard(ctx context.Context, in0 *adminservice.GetShardRequest) (*adminservice.GetShardResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetShard is not allowed.")
}

func (s *mockSenderService) GetTaskQueueTasks(ctx context.Context, in0 *adminservice.GetTaskQueueTasksRequest) (*adminservice.GetTaskQueueTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetTaskQueueTasks is not allowed.")
}

func (s *mockSenderService) GetWorkflowExecutionRawHistory(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryRequest) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionRawHistory is not allowed.")
}

func (s *mockSenderService) GetWorkflowExecutionRawHistoryV2(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryV2Request) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
}

func (s *mockSenderService) ImportWorkflowExecution(ctx context.Context, in0 *adminservice.ImportWorkflowExecutionRequest) (*adminservice.ImportWorkflowExecutionResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ImportWorkflowExecution is not allowed.")
}

func (s *mockSenderService) ListClusterMembers(ctx context.Context, in0 *adminservice.ListClusterMembersRequest) (*adminservice.ListClusterMembersResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusterMembers is not allowed.")
}

func (s *mockSenderService) ListClusters(ctx context.Context, in0 *adminservice.ListClustersRequest) (*adminservice.ListClustersResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusterMembers is not allowed.")
}

func (s *mockSenderService) ListHistoryTasks(ctx context.Context, in0 *adminservice.ListHistoryTasksRequest) (*adminservice.ListHistoryTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListHistoryTasks is not allowed.")
}

func (s *mockSenderService) ListQueues(ctx context.Context, in0 *adminservice.ListQueuesRequest) (*adminservice.ListQueuesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ListQueues is not allowed.")
}

func (s *mockSenderService) MergeDLQMessages(ctx context.Context, in0 *adminservice.MergeDLQMessagesRequest) (*adminservice.MergeDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQMessages is not allowed.")
}

func (s *mockSenderService) MergeDLQTasks(ctx context.Context, in0 *adminservice.MergeDLQTasksRequest) (*adminservice.MergeDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQTasks is not allowed.")
}

func (s *mockSenderService) PurgeDLQMessages(ctx context.Context, in0 *adminservice.PurgeDLQMessagesRequest) (*adminservice.PurgeDLQMessagesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQMessages is not allowed.")
}

func (s *mockSenderService) PurgeDLQTasks(ctx context.Context, in0 *adminservice.PurgeDLQTasksRequest) (*adminservice.PurgeDLQTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQTasks is not allowed.")
}

func (s *mockSenderService) ReapplyEvents(ctx context.Context, in0 *adminservice.ReapplyEventsRequest) (*adminservice.ReapplyEventsResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ReapplyEvents is not allowed.")
}

func (s *mockSenderService) RebuildMutableState(ctx context.Context, in0 *adminservice.RebuildMutableStateRequest) (*adminservice.RebuildMutableStateResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RebuildMutableState is not allowed.")
}

func (s *mockSenderService) RefreshWorkflowTasks(ctx context.Context, in0 *adminservice.RefreshWorkflowTasksRequest) (*adminservice.RefreshWorkflowTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RefreshWorkflowTasks is not allowed.")
}

func (s *mockSenderService) RemoveRemoteCluster(ctx context.Context, in0 *adminservice.RemoveRemoteClusterRequest) (*adminservice.RemoveRemoteClusterResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveRemoteCluster is not allowed.")
}

func (s *mockSenderService) RemoveSearchAttributes(ctx context.Context, in0 *adminservice.RemoveSearchAttributesRequest) (*adminservice.RemoveSearchAttributesResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveSearchAttributes is not allowed.")
}

func (s *mockSenderService) RemoveTask(ctx context.Context, in0 *adminservice.RemoveTaskRequest) (*adminservice.RemoveTaskResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveTask is not allowed.")
}

func (s *mockSenderService) ResendReplicationTasks(ctx context.Context, in0 *adminservice.ResendReplicationTasksRequest) (*adminservice.ResendReplicationTasksResponse, error) {
	return nil, status.Errorf(codes.PermissionDenied, "Calling method ResendReplicationTasks is not allowed.")
}

func toString(sd history.ClusterShardID) string {
	return fmt.Sprintf("(id: %d, shard: %d)", sd.ClusterID, sd.ShardID)
}

func (s *mockSenderService) StreamWorkflowReplicationMessages(
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
		common.ServiceTag(s.serviceName),
		tag.NewStringTag("target", toString(targetClusterShardID)),
		tag.NewStringTag("source", toString(sourceClusterShardID)))

	logger.Info("AdminStreamReplicationMessages started.")
	defer logger.Info("AdminStreamReplicationMessages stopped.")

	go func() {
		for {
			req, err := targetStreamServer.Recv()
			if err == io.EOF {
				return
			}

			if err != nil {
				logger.Error("targetStreamServer.Recv encountered error", tag.Error(err))
				return
			}

			switch attr := req.GetAttributes().(type) {
			case *adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState:
				logger.Debug(fmt.Sprintf("forwarding SyncReplicationState: inclusive %v", attr.SyncReplicationState.InclusiveLowWatermark))

				// do something here

			default:
				logger.Error("targetStreamServer.Recv encountered error", tag.Error(serviceerror.NewInternal(fmt.Sprintf(
					"StreamWorkflowReplicationMessages encountered unknown type: %T %v", attr, attr,
				))))
				return
			}
		}
	}()
	return nil
}
