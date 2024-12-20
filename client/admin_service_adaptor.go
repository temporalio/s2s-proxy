package client

import (
	"context"
	"github.com/temporalio/s2s-proxy/proxy/cadence/cadencetype"
	"github.com/temporalio/s2s-proxy/proxy/cadence/temporaltype"
	adminv1 "github.com/uber/cadence-idl/go/proto/admin/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
)

type adminServiceAdaptor struct {
	logger        log.Logger
	cadenceClient adminv1.AdminAPIYARPCClient
}

var _ adminservice.AdminServiceClient = adminServiceAdaptor{}

func NewAdminServiceAdaptor(logger log.Logger, cadenceClient adminv1.AdminAPIYARPCClient) adminservice.AdminServiceClient {
	return adminServiceAdaptor{
		logger:        logger,
		cadenceClient: cadenceClient,
	}
}

func (a adminServiceAdaptor) RebuildMutableState(ctx context.Context, in *adminservice.RebuildMutableStateRequest, opts ...grpc.CallOption) (*adminservice.RebuildMutableStateResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) ImportWorkflowExecution(ctx context.Context, in *adminservice.ImportWorkflowExecutionRequest, opts ...grpc.CallOption) (*adminservice.ImportWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) DescribeMutableState(ctx context.Context, in *adminservice.DescribeMutableStateRequest, opts ...grpc.CallOption) (*adminservice.DescribeMutableStateResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) DescribeHistoryHost(ctx context.Context, in *adminservice.DescribeHistoryHostRequest, opts ...grpc.CallOption) (*adminservice.DescribeHistoryHostResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetShard(ctx context.Context, in *adminservice.GetShardRequest, opts ...grpc.CallOption) (*adminservice.GetShardResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) CloseShard(ctx context.Context, in *adminservice.CloseShardRequest, opts ...grpc.CallOption) (*adminservice.CloseShardResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) ListHistoryTasks(ctx context.Context, in *adminservice.ListHistoryTasksRequest, opts ...grpc.CallOption) (*adminservice.ListHistoryTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) RemoveTask(ctx context.Context, in *adminservice.RemoveTaskRequest, opts ...grpc.CallOption) (*adminservice.RemoveTaskResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetWorkflowExecutionRawHistoryV2(ctx context.Context, in *adminservice.GetWorkflowExecutionRawHistoryV2Request, opts ...grpc.CallOption) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetWorkflowExecutionRawHistory(ctx context.Context, in *adminservice.GetWorkflowExecutionRawHistoryRequest, opts ...grpc.CallOption) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetReplicationMessages(ctx context.Context, in *adminservice.GetReplicationMessagesRequest, opts ...grpc.CallOption) (*adminservice.GetReplicationMessagesResponse, error) {
	// a.logger.Debug("Cadence client: GetReplicationMessages called.")

	tReq := cadencetype.GetReplicationMessagesRequest(in)
	resp, err := a.cadenceClient.GetReplicationMessages(ctx, tReq)
	return temporaltype.GetReplicationMessagesResponse(resp), err
}

func (a adminServiceAdaptor) GetNamespaceReplicationMessages(ctx context.Context, in *adminservice.GetNamespaceReplicationMessagesRequest, opts ...grpc.CallOption) (*adminservice.GetNamespaceReplicationMessagesResponse, error) {
	//a.logger.Debug("Cadence client: GetNamespaceReplicationMessages called.")

	tReq := cadencetype.GetNamespaceReplicationMessagesRequest(in)
	resp, err := a.cadenceClient.GetDomainReplicationMessages(ctx, tReq)
	return temporaltype.GetNamespaceReplicationMessagesResponse(resp), err
}

func (a adminServiceAdaptor) GetDLQReplicationMessages(ctx context.Context, in *adminservice.GetDLQReplicationMessagesRequest, opts ...grpc.CallOption) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) ReapplyEvents(ctx context.Context, in *adminservice.ReapplyEventsRequest, opts ...grpc.CallOption) (*adminservice.ReapplyEventsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) AddSearchAttributes(ctx context.Context, in *adminservice.AddSearchAttributesRequest, opts ...grpc.CallOption) (*adminservice.AddSearchAttributesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) RemoveSearchAttributes(ctx context.Context, in *adminservice.RemoveSearchAttributesRequest, opts ...grpc.CallOption) (*adminservice.RemoveSearchAttributesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetSearchAttributes(ctx context.Context, in *adminservice.GetSearchAttributesRequest, opts ...grpc.CallOption) (*adminservice.GetSearchAttributesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) DescribeCluster(ctx context.Context, in *adminservice.DescribeClusterRequest, opts ...grpc.CallOption) (*adminservice.DescribeClusterResponse, error) {
	a.logger.Info("Cadence client: DescribeCluster called.")

	tReq := cadencetype.DescribeClusterRequest(in)
	resp, err := a.cadenceClient.DescribeCluster(ctx, tReq)
	return temporaltype.DescribeClusterResponse(resp), err
}

func (a adminServiceAdaptor) ListClusters(ctx context.Context, in *adminservice.ListClustersRequest, opts ...grpc.CallOption) (*adminservice.ListClustersResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) ListClusterMembers(ctx context.Context, in *adminservice.ListClusterMembersRequest, opts ...grpc.CallOption) (*adminservice.ListClusterMembersResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) AddOrUpdateRemoteCluster(ctx context.Context, in *adminservice.AddOrUpdateRemoteClusterRequest, opts ...grpc.CallOption) (*adminservice.AddOrUpdateRemoteClusterResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) RemoveRemoteCluster(ctx context.Context, in *adminservice.RemoveRemoteClusterRequest, opts ...grpc.CallOption) (*adminservice.RemoveRemoteClusterResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetDLQMessages(ctx context.Context, in *adminservice.GetDLQMessagesRequest, opts ...grpc.CallOption) (*adminservice.GetDLQMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) PurgeDLQMessages(ctx context.Context, in *adminservice.PurgeDLQMessagesRequest, opts ...grpc.CallOption) (*adminservice.PurgeDLQMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) MergeDLQMessages(ctx context.Context, in *adminservice.MergeDLQMessagesRequest, opts ...grpc.CallOption) (*adminservice.MergeDLQMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) RefreshWorkflowTasks(ctx context.Context, in *adminservice.RefreshWorkflowTasksRequest, opts ...grpc.CallOption) (*adminservice.RefreshWorkflowTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) ResendReplicationTasks(ctx context.Context, in *adminservice.ResendReplicationTasksRequest, opts ...grpc.CallOption) (*adminservice.ResendReplicationTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetTaskQueueTasks(ctx context.Context, in *adminservice.GetTaskQueueTasksRequest, opts ...grpc.CallOption) (*adminservice.GetTaskQueueTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) DeleteWorkflowExecution(ctx context.Context, in *adminservice.DeleteWorkflowExecutionRequest, opts ...grpc.CallOption) (*adminservice.DeleteWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) StreamWorkflowReplicationMessages(ctx context.Context, opts ...grpc.CallOption) (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetNamespace(ctx context.Context, in *adminservice.GetNamespaceRequest, opts ...grpc.CallOption) (*adminservice.GetNamespaceResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) GetDLQTasks(ctx context.Context, in *adminservice.GetDLQTasksRequest, opts ...grpc.CallOption) (*adminservice.GetDLQTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) PurgeDLQTasks(ctx context.Context, in *adminservice.PurgeDLQTasksRequest, opts ...grpc.CallOption) (*adminservice.PurgeDLQTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) MergeDLQTasks(ctx context.Context, in *adminservice.MergeDLQTasksRequest, opts ...grpc.CallOption) (*adminservice.MergeDLQTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) DescribeDLQJob(ctx context.Context, in *adminservice.DescribeDLQJobRequest, opts ...grpc.CallOption) (*adminservice.DescribeDLQJobResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) CancelDLQJob(ctx context.Context, in *adminservice.CancelDLQJobRequest, opts ...grpc.CallOption) (*adminservice.CancelDLQJobResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) AddTasks(ctx context.Context, in *adminservice.AddTasksRequest, opts ...grpc.CallOption) (*adminservice.AddTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceAdaptor) ListQueues(ctx context.Context, in *adminservice.ListQueuesRequest, opts ...grpc.CallOption) (*adminservice.ListQueuesResponse, error) {
	//TODO implement me
	panic("implement me")
}
