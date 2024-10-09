package proxy

import (
	"context"

	"github.com/gogo/status"
	"github.com/temporalio/s2s-proxy/auth"
	"go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/grpc/codes"
)

type (
	adminServiceAuth struct {
		adminservice.UnimplementedAdminServiceServer
		delegate adminservice.AdminServiceServer
		access   *auth.AccessControl
	}
)

func newAdminServiceAuth(
	delegate adminservice.AdminServiceServer,
	access *auth.AccessControl,
) adminservice.AdminServiceServer {
	return &adminServiceAuth{
		delegate: delegate,
		access:   access,
	}
}

func (s *adminServiceAuth) AddOrUpdateRemoteCluster(ctx context.Context, in0 *adminservice.AddOrUpdateRemoteClusterRequest) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	if !s.access.IsAllowed("AddOrUpdateRemoteCluster") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method AddOrUpdateRemoteCluster is not allowed.")
	}

	return s.delegate.AddOrUpdateRemoteCluster(ctx, in0)
}

func (s *adminServiceAuth) AddSearchAttributes(ctx context.Context, in0 *adminservice.AddSearchAttributesRequest) (*adminservice.AddSearchAttributesResponse, error) {
	if !s.access.IsAllowed("AddSearchAttributes") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method AddSearchAttributes is not allowed.")
	}

	return s.delegate.AddSearchAttributes(ctx, in0)
}

func (s *adminServiceAuth) AddTasks(ctx context.Context, in0 *adminservice.AddTasksRequest) (*adminservice.AddTasksResponse, error) {
	if !s.access.IsAllowed("AddTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method AddTasks is not allowed.")
	}

	return s.delegate.AddTasks(ctx, in0)
}

func (s *adminServiceAuth) CancelDLQJob(ctx context.Context, in0 *adminservice.CancelDLQJobRequest) (*adminservice.CancelDLQJobResponse, error) {
	if !s.access.IsAllowed("CancelDLQJob") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method CancelDLQJob is not allowed.")
	}

	return s.delegate.CancelDLQJob(ctx, in0)
}

func (s *adminServiceAuth) CloseShard(ctx context.Context, in0 *adminservice.CloseShardRequest) (*adminservice.CloseShardResponse, error) {
	if !s.access.IsAllowed("CloseShard") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method CloseShard is not allowed.")
	}

	return s.delegate.CloseShard(ctx, in0)
}

func (s *adminServiceAuth) DeleteWorkflowExecution(ctx context.Context, in0 *adminservice.DeleteWorkflowExecutionRequest) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	if !s.access.IsAllowed("DeleteWorkflowExecution") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DeleteWorkflowExecution is not allowed.")
	}

	return s.delegate.DeleteWorkflowExecution(ctx, in0)
}

func (s *adminServiceAuth) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	if !s.access.IsAllowed("DescribeCluster") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeCluster is not allowed.")
	}

	return s.delegate.DescribeCluster(ctx, in0)
}

func (s *adminServiceAuth) DescribeDLQJob(ctx context.Context, in0 *adminservice.DescribeDLQJobRequest) (*adminservice.DescribeDLQJobResponse, error) {
	if !s.access.IsAllowed("DescribeDLQJob") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeDLQJob is not allowed.")
	}

	return s.delegate.DescribeDLQJob(ctx, in0)
}

func (s *adminServiceAuth) DescribeHistoryHost(ctx context.Context, in0 *adminservice.DescribeHistoryHostRequest) (*adminservice.DescribeHistoryHostResponse, error) {
	if !s.access.IsAllowed("DescribeHistoryHost") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeHistoryHost is not allowed.")
	}

	return s.delegate.DescribeHistoryHost(ctx, in0)
}

func (s *adminServiceAuth) DescribeMutableState(ctx context.Context, in0 *adminservice.DescribeMutableStateRequest) (*adminservice.DescribeMutableStateResponse, error) {
	if !s.access.IsAllowed("DescribeMutableState") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method DescribeMutableState is not allowed.")
	}

	return s.delegate.DescribeMutableState(ctx, in0)
}

func (s *adminServiceAuth) GetDLQMessages(ctx context.Context, in0 *adminservice.GetDLQMessagesRequest) (*adminservice.GetDLQMessagesResponse, error) {
	if !s.access.IsAllowed("GetDLQMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQMessages is not allowed.")
	}

	return s.delegate.GetDLQMessages(ctx, in0)
}

func (s *adminServiceAuth) GetDLQReplicationMessages(ctx context.Context, in0 *adminservice.GetDLQReplicationMessagesRequest) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	if !s.access.IsAllowed("GetDLQReplicationMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQReplicationMessages is not allowed.")
	}

	return s.delegate.GetDLQReplicationMessages(ctx, in0)
}

func (s *adminServiceAuth) GetDLQTasks(ctx context.Context, in0 *adminservice.GetDLQTasksRequest) (*adminservice.GetDLQTasksResponse, error) {
	if !s.access.IsAllowed("GetDLQTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetDLQTasks is not allowed.")
	}

	return s.delegate.GetDLQTasks(ctx, in0)
}

func (s *adminServiceAuth) GetNamespace(ctx context.Context, in0 *adminservice.GetNamespaceRequest) (*adminservice.GetNamespaceResponse, error) {
	if !s.access.IsAllowed("GetNamespace") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetNamespace is not allowed.")
	}

	return s.delegate.GetNamespace(ctx, in0)
}

func (s *adminServiceAuth) GetNamespaceReplicationMessages(ctx context.Context, in0 *adminservice.GetNamespaceReplicationMessagesRequest) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	if !s.access.IsAllowed("GetNamespaceReplicationMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetNamespaceReplicationMessages is not allowed.")
	}

	return s.delegate.GetNamespaceReplicationMessages(ctx, in0)
}

func (s *adminServiceAuth) GetReplicationMessages(ctx context.Context, in0 *adminservice.GetReplicationMessagesRequest) (*adminservice.GetReplicationMessagesResponse, error) {
	if !s.access.IsAllowed("GetReplicationMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetReplicationMessages is not allowed.")
	}

	return s.delegate.GetReplicationMessages(ctx, in0)
}

func (s *adminServiceAuth) GetSearchAttributes(ctx context.Context, in0 *adminservice.GetSearchAttributesRequest) (*adminservice.GetSearchAttributesResponse, error) {
	if !s.access.IsAllowed("GetSearchAttributes") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetSearchAttributes is not allowed.")
	}

	return s.delegate.GetSearchAttributes(ctx, in0)
}

func (s *adminServiceAuth) GetShard(ctx context.Context, in0 *adminservice.GetShardRequest) (*adminservice.GetShardResponse, error) {
	if !s.access.IsAllowed("GetShard") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetShard is not allowed.")
	}

	return s.delegate.GetShard(ctx, in0)
}

func (s *adminServiceAuth) GetTaskQueueTasks(ctx context.Context, in0 *adminservice.GetTaskQueueTasksRequest) (*adminservice.GetTaskQueueTasksResponse, error) {
	if !s.access.IsAllowed("GetTaskQueueTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetTaskQueueTasks is not allowed.")
	}

	return s.delegate.GetTaskQueueTasks(ctx, in0)
}

func (s *adminServiceAuth) GetWorkflowExecutionRawHistory(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryRequest) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	if !s.access.IsAllowed("GetWorkflowExecutionRawHistory") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionRawHistory is not allowed.")
	}

	return s.delegate.GetWorkflowExecutionRawHistory(ctx, in0)
}

func (s *adminServiceAuth) GetWorkflowExecutionRawHistoryV2(ctx context.Context, in0 *adminservice.GetWorkflowExecutionRawHistoryV2Request) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	if !s.access.IsAllowed("GetWorkflowExecutionRawHistoryV2") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method GetWorkflowExecutionRawHistoryV2 is not allowed.")
	}

	return s.delegate.GetWorkflowExecutionRawHistoryV2(ctx, in0)
}

func (s *adminServiceAuth) ImportWorkflowExecution(ctx context.Context, in0 *adminservice.ImportWorkflowExecutionRequest) (*adminservice.ImportWorkflowExecutionResponse, error) {
	if !s.access.IsAllowed("ImportWorkflowExecution") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ImportWorkflowExecution is not allowed.")
	}

	return s.delegate.ImportWorkflowExecution(ctx, in0)
}

func (s *adminServiceAuth) ListClusterMembers(ctx context.Context, in0 *adminservice.ListClusterMembersRequest) (*adminservice.ListClusterMembersResponse, error) {
	if !s.access.IsAllowed("ListClusterMembers") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusterMembers is not allowed.")
	}

	return s.delegate.ListClusterMembers(ctx, in0)
}

func (s *adminServiceAuth) ListClusters(ctx context.Context, in0 *adminservice.ListClustersRequest) (*adminservice.ListClustersResponse, error) {
	if !s.access.IsAllowed("ListClusters") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ListClusters is not allowed.")
	}

	return s.delegate.ListClusters(ctx, in0)
}

func (s *adminServiceAuth) ListHistoryTasks(ctx context.Context, in0 *adminservice.ListHistoryTasksRequest) (*adminservice.ListHistoryTasksResponse, error) {
	if !s.access.IsAllowed("ListHistoryTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ListHistoryTasks is not allowed.")
	}

	return s.delegate.ListHistoryTasks(ctx, in0)
}

func (s *adminServiceAuth) ListQueues(ctx context.Context, in0 *adminservice.ListQueuesRequest) (*adminservice.ListQueuesResponse, error) {
	if !s.access.IsAllowed("ListQueues") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ListQueues is not allowed.")
	}

	return s.delegate.ListQueues(ctx, in0)
}

func (s *adminServiceAuth) MergeDLQMessages(ctx context.Context, in0 *adminservice.MergeDLQMessagesRequest) (*adminservice.MergeDLQMessagesResponse, error) {
	if !s.access.IsAllowed("MergeDLQMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQMessages is not allowed.")
	}

	return s.delegate.MergeDLQMessages(ctx, in0)
}

func (s *adminServiceAuth) MergeDLQTasks(ctx context.Context, in0 *adminservice.MergeDLQTasksRequest) (*adminservice.MergeDLQTasksResponse, error) {
	if !s.access.IsAllowed("MergeDLQTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method MergeDLQTasks is not allowed.")
	}

	return s.delegate.MergeDLQTasks(ctx, in0)
}

func (s *adminServiceAuth) PurgeDLQMessages(ctx context.Context, in0 *adminservice.PurgeDLQMessagesRequest) (*adminservice.PurgeDLQMessagesResponse, error) {
	if !s.access.IsAllowed("PurgeDLQMessages") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQMessages is not allowed.")
	}

	return s.delegate.PurgeDLQMessages(ctx, in0)
}

func (s *adminServiceAuth) PurgeDLQTasks(ctx context.Context, in0 *adminservice.PurgeDLQTasksRequest) (*adminservice.PurgeDLQTasksResponse, error) {
	if !s.access.IsAllowed("PurgeDLQTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method PurgeDLQTasks is not allowed.")
	}

	return s.delegate.PurgeDLQTasks(ctx, in0)
}

func (s *adminServiceAuth) ReapplyEvents(ctx context.Context, in0 *adminservice.ReapplyEventsRequest) (*adminservice.ReapplyEventsResponse, error) {
	if !s.access.IsAllowed("ReapplyEvents") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ReapplyEvents is not allowed.")
	}

	return s.delegate.ReapplyEvents(ctx, in0)
}

func (s *adminServiceAuth) RebuildMutableState(ctx context.Context, in0 *adminservice.RebuildMutableStateRequest) (*adminservice.RebuildMutableStateResponse, error) {
	if !s.access.IsAllowed("RebuildMutableState") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RebuildMutableState is not allowed.")
	}

	return s.delegate.RebuildMutableState(ctx, in0)
}

func (s *adminServiceAuth) RefreshWorkflowTasks(ctx context.Context, in0 *adminservice.RefreshWorkflowTasksRequest) (*adminservice.RefreshWorkflowTasksResponse, error) {
	if !s.access.IsAllowed("RefreshWorkflowTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RefreshWorkflowTasks is not allowed.")
	}

	return s.delegate.RefreshWorkflowTasks(ctx, in0)
}

func (s *adminServiceAuth) RemoveRemoteCluster(ctx context.Context, in0 *adminservice.RemoveRemoteClusterRequest) (*adminservice.RemoveRemoteClusterResponse, error) {
	if !s.access.IsAllowed("RemoveRemoteCluster") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveRemoteCluster is not allowed.")
	}

	return s.delegate.RemoveRemoteCluster(ctx, in0)
}

func (s *adminServiceAuth) RemoveSearchAttributes(ctx context.Context, in0 *adminservice.RemoveSearchAttributesRequest) (*adminservice.RemoveSearchAttributesResponse, error) {
	if !s.access.IsAllowed("RemoveSearchAttributes") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveSearchAttributes is not allowed.")
	}

	return s.delegate.RemoveSearchAttributes(ctx, in0)
}

func (s *adminServiceAuth) RemoveTask(ctx context.Context, in0 *adminservice.RemoveTaskRequest) (*adminservice.RemoveTaskResponse, error) {
	if !s.access.IsAllowed("RemoveTask") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method RemoveTask is not allowed.")
	}

	return s.delegate.RemoveTask(ctx, in0)
}

func (s *adminServiceAuth) ResendReplicationTasks(ctx context.Context, in0 *adminservice.ResendReplicationTasksRequest) (*adminservice.ResendReplicationTasksResponse, error) {
	if !s.access.IsAllowed("ResendReplicationTasks") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method ResendReplicationTasks is not allowed.")
	}

	return s.delegate.ResendReplicationTasks(ctx, in0)
}
func (s *adminServiceAuth) StreamWorkflowReplicationMessages(
	in0 adminservice.AdminService_StreamWorkflowReplicationMessagesServer,
) (retError error) {
	if !s.access.IsAllowed("StreamWorkflowReplicationMessages") {
		return status.Errorf(codes.PermissionDenied, "Calling method StreamWorkflowReplicationMessages is not allowed.")
	}

	return s.delegate.StreamWorkflowReplicationMessages(in0)
}
