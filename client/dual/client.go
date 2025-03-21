package dual

import (
	"context"

	"github.com/temporalio/s2s-proxy/client"
	adminclient "github.com/temporalio/s2s-proxy/client/admin"
	operatorclient "github.com/temporalio/s2s-proxy/client/operator"
	"google.golang.org/grpc"

	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
)

type (
	routingClient struct {
		adminClient    adminservice.AdminServiceClient
		operatorClient operatorservice.OperatorServiceClient
	}
)

func NewRoutingClient(clientProvider client.ClientProvider) *routingClient {
	return &routingClient{
		adminClient:    adminclient.NewLazyClient(clientProvider),
		operatorClient: operatorclient.NewLazyClient(clientProvider),
	}
}

func (c *routingClient) AddOrUpdateRemoteCluster(
	ctx context.Context,
	request *adminservice.AddOrUpdateRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	operatorRequest := convertAddOrUpdateRemoteClusterRequest(request)
	operatorResponse, err := c.operatorClient.AddOrUpdateRemoteCluster(ctx, operatorRequest, opts...)
	if err != nil {
		return nil, err
	}
	return convertAddOrUpdateRemoteClusterResponse(operatorResponse), nil
}

func (c *routingClient) AddSearchAttributes(
	ctx context.Context,
	request *adminservice.AddSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.AddSearchAttributesResponse, error) {
	return c.adminClient.AddSearchAttributes(ctx, request, opts...)
}

func (c *routingClient) AddTasks(
	ctx context.Context,
	request *adminservice.AddTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.AddTasksResponse, error) {
	return c.adminClient.AddTasks(ctx, request, opts...)
}

func (c *routingClient) CancelDLQJob(
	ctx context.Context,
	request *adminservice.CancelDLQJobRequest,
	opts ...grpc.CallOption,
) (*adminservice.CancelDLQJobResponse, error) {
	return c.adminClient.CancelDLQJob(ctx, request, opts...)
}

func (c *routingClient) CloseShard(
	ctx context.Context,
	request *adminservice.CloseShardRequest,
	opts ...grpc.CallOption,
) (*adminservice.CloseShardResponse, error) {
	return c.adminClient.CloseShard(ctx, request, opts...)
}

func (c *routingClient) DeepHealthCheck(
	ctx context.Context,
	request *adminservice.DeepHealthCheckRequest,
	opts ...grpc.CallOption,
) (*adminservice.DeepHealthCheckResponse, error) {
	return c.adminClient.DeepHealthCheck(ctx, request, opts...)
}

func (c *routingClient) DeleteWorkflowExecution(
	ctx context.Context,
	request *adminservice.DeleteWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	return c.adminClient.DeleteWorkflowExecution(ctx, request, opts...)
}

func (c *routingClient) DescribeCluster(
	ctx context.Context,
	request *adminservice.DescribeClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeClusterResponse, error) {
	return c.adminClient.DescribeCluster(ctx, request, opts...)
}

func (c *routingClient) DescribeDLQJob(
	ctx context.Context,
	request *adminservice.DescribeDLQJobRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeDLQJobResponse, error) {
	return c.adminClient.DescribeDLQJob(ctx, request, opts...)
}

func (c *routingClient) DescribeHistoryHost(
	ctx context.Context,
	request *adminservice.DescribeHistoryHostRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeHistoryHostResponse, error) {
	return c.adminClient.DescribeHistoryHost(ctx, request, opts...)
}

func (c *routingClient) DescribeMutableState(
	ctx context.Context,
	request *adminservice.DescribeMutableStateRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeMutableStateResponse, error) {
	return c.adminClient.DescribeMutableState(ctx, request, opts...)
}

func (c *routingClient) DescribeTaskQueuePartition(
	ctx context.Context,
	request *adminservice.DescribeTaskQueuePartitionRequest,
	opts ...grpc.CallOption,
) (*adminservice.DescribeTaskQueuePartitionResponse, error) {
	return c.adminClient.DescribeTaskQueuePartition(ctx, request, opts...)
}

func (c *routingClient) ForceUnloadTaskQueuePartition(
	ctx context.Context,
	request *adminservice.ForceUnloadTaskQueuePartitionRequest,
	opts ...grpc.CallOption,
) (*adminservice.ForceUnloadTaskQueuePartitionResponse, error) {
	return c.adminClient.ForceUnloadTaskQueuePartition(ctx, request, opts...)
}

func (c *routingClient) GenerateLastHistoryReplicationTasks(
	ctx context.Context,
	request *adminservice.GenerateLastHistoryReplicationTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.GenerateLastHistoryReplicationTasksResponse, error) {
	return c.adminClient.GenerateLastHistoryReplicationTasks(ctx, request, opts...)
}

func (c *routingClient) GetDLQMessages(
	ctx context.Context,
	request *adminservice.GetDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetDLQMessagesResponse, error) {
	return c.adminClient.GetDLQMessages(ctx, request, opts...)
}

func (c *routingClient) GetDLQReplicationMessages(
	ctx context.Context,
	request *adminservice.GetDLQReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	return c.adminClient.GetDLQReplicationMessages(ctx, request, opts...)
}

func (c *routingClient) GetDLQTasks(
	ctx context.Context,
	request *adminservice.GetDLQTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetDLQTasksResponse, error) {
	return c.adminClient.GetDLQTasks(ctx, request, opts...)
}

func (c *routingClient) GetNamespace(
	ctx context.Context,
	request *adminservice.GetNamespaceRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetNamespaceResponse, error) {
	return c.adminClient.GetNamespace(ctx, request, opts...)
}

func (c *routingClient) GetNamespaceReplicationMessages(
	ctx context.Context,
	request *adminservice.GetNamespaceReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	return c.adminClient.GetNamespaceReplicationMessages(ctx, request, opts...)
}

func (c *routingClient) GetReplicationMessages(
	ctx context.Context,
	request *adminservice.GetReplicationMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetReplicationMessagesResponse, error) {
	return c.adminClient.GetReplicationMessages(ctx, request, opts...)
}

func (c *routingClient) GetSearchAttributes(
	ctx context.Context,
	request *adminservice.GetSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetSearchAttributesResponse, error) {
	return c.adminClient.GetSearchAttributes(ctx, request, opts...)
}

func (c *routingClient) GetShard(
	ctx context.Context,
	request *adminservice.GetShardRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetShardResponse, error) {
	return c.adminClient.GetShard(ctx, request, opts...)
}

func (c *routingClient) GetTaskQueueTasks(
	ctx context.Context,
	request *adminservice.GetTaskQueueTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetTaskQueueTasksResponse, error) {
	return c.adminClient.GetTaskQueueTasks(ctx, request, opts...)
}

func (c *routingClient) GetWorkflowExecutionRawHistory(
	ctx context.Context,
	request *adminservice.GetWorkflowExecutionRawHistoryRequest,
	opts ...grpc.CallOption,
) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	return c.adminClient.GetWorkflowExecutionRawHistory(ctx, request, opts...)
}

func (c *routingClient) GetWorkflowExecutionRawHistoryV2(
	ctx context.Context,
	request *adminservice.GetWorkflowExecutionRawHistoryV2Request,
	opts ...grpc.CallOption,
) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	return c.adminClient.GetWorkflowExecutionRawHistoryV2(ctx, request, opts...)
}

func (c *routingClient) ImportWorkflowExecution(
	ctx context.Context,
	request *adminservice.ImportWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*adminservice.ImportWorkflowExecutionResponse, error) {
	return c.adminClient.ImportWorkflowExecution(ctx, request, opts...)
}

func (c *routingClient) ListClusterMembers(
	ctx context.Context,
	request *adminservice.ListClusterMembersRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListClusterMembersResponse, error) {
	return c.adminClient.ListClusterMembers(ctx, request, opts...)
}

func (c *routingClient) ListClusters(
	ctx context.Context,
	request *adminservice.ListClustersRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListClustersResponse, error) {
	operatorRequest := convertListClustersRequest(request)
	operatorResponse, err := c.operatorClient.ListClusters(ctx, operatorRequest, opts...)
	if err != nil {
		return nil, err
	}
	return convertListClustersResponse(operatorResponse), nil
}
func (c *routingClient) ListHistoryTasks(
	ctx context.Context,
	request *adminservice.ListHistoryTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListHistoryTasksResponse, error) {
	return c.adminClient.ListHistoryTasks(ctx, request, opts...)
}

func (c *routingClient) ListQueues(
	ctx context.Context,
	request *adminservice.ListQueuesRequest,
	opts ...grpc.CallOption,
) (*adminservice.ListQueuesResponse, error) {
	return c.adminClient.ListQueues(ctx, request, opts...)
}

func (c *routingClient) MergeDLQMessages(
	ctx context.Context,
	request *adminservice.MergeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.MergeDLQMessagesResponse, error) {
	return c.adminClient.MergeDLQMessages(ctx, request, opts...)
}

func (c *routingClient) MergeDLQTasks(
	ctx context.Context,
	request *adminservice.MergeDLQTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.MergeDLQTasksResponse, error) {
	return c.adminClient.MergeDLQTasks(ctx, request, opts...)
}

func (c *routingClient) PurgeDLQMessages(
	ctx context.Context,
	request *adminservice.PurgeDLQMessagesRequest,
	opts ...grpc.CallOption,
) (*adminservice.PurgeDLQMessagesResponse, error) {
	return c.adminClient.PurgeDLQMessages(ctx, request, opts...)
}

func (c *routingClient) PurgeDLQTasks(
	ctx context.Context,
	request *adminservice.PurgeDLQTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.PurgeDLQTasksResponse, error) {
	return c.adminClient.PurgeDLQTasks(ctx, request, opts...)
}

func (c *routingClient) ReapplyEvents(
	ctx context.Context,
	request *adminservice.ReapplyEventsRequest,
	opts ...grpc.CallOption,
) (*adminservice.ReapplyEventsResponse, error) {
	return c.adminClient.ReapplyEvents(ctx, request, opts...)
}

func (c *routingClient) RebuildMutableState(
	ctx context.Context,
	request *adminservice.RebuildMutableStateRequest,
	opts ...grpc.CallOption,
) (*adminservice.RebuildMutableStateResponse, error) {
	return c.adminClient.RebuildMutableState(ctx, request, opts...)
}

func (c *routingClient) RefreshWorkflowTasks(
	ctx context.Context,
	request *adminservice.RefreshWorkflowTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.RefreshWorkflowTasksResponse, error) {
	return c.adminClient.RefreshWorkflowTasks(ctx, request, opts...)
}

func (c *routingClient) RemoveRemoteCluster(
	ctx context.Context,
	request *adminservice.RemoveRemoteClusterRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveRemoteClusterResponse, error) {
	operatorRequest := convertRemoveRemoteClusterRequest(request)
	operatorResponse, err := c.operatorClient.RemoveRemoteCluster(ctx, operatorRequest, opts...)
	if err != nil {
		return nil, err
	}
	return convertRemoveRemoteClusterResponse(operatorResponse), nil
}

func (c *routingClient) RemoveSearchAttributes(
	ctx context.Context,
	request *adminservice.RemoveSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveSearchAttributesResponse, error) {
	return c.adminClient.RemoveSearchAttributes(ctx, request, opts...)
}

func (c *routingClient) RemoveTask(
	ctx context.Context,
	request *adminservice.RemoveTaskRequest,
	opts ...grpc.CallOption,
) (*adminservice.RemoveTaskResponse, error) {
	return c.adminClient.RemoveTask(ctx, request, opts...)
}

func (c *routingClient) ResendReplicationTasks(
	ctx context.Context,
	request *adminservice.ResendReplicationTasksRequest,
	opts ...grpc.CallOption,
) (*adminservice.ResendReplicationTasksResponse, error) {
	return c.adminClient.ResendReplicationTasks(ctx, request, opts...)
}

func (c *routingClient) SyncWorkflowState(
	ctx context.Context,
	request *adminservice.SyncWorkflowStateRequest,
	opts ...grpc.CallOption,
) (*adminservice.SyncWorkflowStateResponse, error) {
	return c.adminClient.SyncWorkflowState(ctx, request, opts...)
}

func (c *routingClient) StreamWorkflowReplicationMessages(
	ctx context.Context,
	opts ...grpc.CallOption,
) (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
	return c.adminClient.StreamWorkflowReplicationMessages(ctx, opts...)
}
