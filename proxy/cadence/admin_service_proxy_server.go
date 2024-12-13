package cadence

import (
	"context"
	"github.com/temporalio/s2s-proxy/proxy/cadence/cadencetype"
	"github.com/temporalio/s2s-proxy/proxy/cadence/temporaltype"
	adminv1 "github.com/uber/cadence-idl/go/proto/admin/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
)

var _ adminv1.AdminAPIYARPCServer = adminServiceProxyServer{}

type adminServiceProxyServer struct {
	logger      log.Logger
	adminClient adminservice.AdminServiceClient
}

func NewAdminServiceProxyServer(logger log.Logger, adminClient adminservice.AdminServiceClient) adminv1.AdminAPIYARPCServer {
	return adminServiceProxyServer{
		logger:      logger,
		adminClient: adminClient,
	}
}

func (a adminServiceProxyServer) DescribeWorkflowExecution(ctx context.Context, request *adminv1.DescribeWorkflowExecutionRequest) (*adminv1.DescribeWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) DescribeHistoryHost(ctx context.Context, request *adminv1.DescribeHistoryHostRequest) (*adminv1.DescribeHistoryHostResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) DescribeShardDistribution(ctx context.Context, request *adminv1.DescribeShardDistributionRequest) (*adminv1.DescribeShardDistributionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) CloseShard(ctx context.Context, request *adminv1.CloseShardRequest) (*adminv1.CloseShardResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) RemoveTask(ctx context.Context, request *adminv1.RemoveTaskRequest) (*adminv1.RemoveTaskResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) ResetQueue(ctx context.Context, request *adminv1.ResetQueueRequest) (*adminv1.ResetQueueResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) DescribeQueue(ctx context.Context, request *adminv1.DescribeQueueRequest) (*adminv1.DescribeQueueResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) GetWorkflowExecutionRawHistoryV2(ctx context.Context, request *adminv1.GetWorkflowExecutionRawHistoryV2Request) (*adminv1.GetWorkflowExecutionRawHistoryV2Response, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) GetReplicationMessages(ctx context.Context, request *adminv1.GetReplicationMessagesRequest) (*adminv1.GetReplicationMessagesResponse, error) {
	a.logger.Info("Admin Proxy: GetReplicationMessages called.")

	tReq := temporaltype.GetReplicationMessagesRequest(request)
	resp, err := a.adminClient.GetReplicationMessages(ctx, tReq)
	return cadencetype.GetReplicationMessagesResponse(resp), cadencetype.Error(err)
}

func (a adminServiceProxyServer) GetDLQReplicationMessages(ctx context.Context, request *adminv1.GetDLQReplicationMessagesRequest) (*adminv1.GetDLQReplicationMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) GetDomainReplicationMessages(ctx context.Context, request *adminv1.GetDomainReplicationMessagesRequest) (*adminv1.GetDomainReplicationMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) ReapplyEvents(ctx context.Context, request *adminv1.ReapplyEventsRequest) (*adminv1.ReapplyEventsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) AddSearchAttribute(ctx context.Context, request *adminv1.AddSearchAttributeRequest) (*adminv1.AddSearchAttributeResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) DescribeCluster(ctx context.Context, request *adminv1.DescribeClusterRequest) (*adminv1.DescribeClusterResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) CountDLQMessages(ctx context.Context, request *adminv1.CountDLQMessagesRequest) (*adminv1.CountDLQMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) ReadDLQMessages(ctx context.Context, request *adminv1.ReadDLQMessagesRequest) (*adminv1.ReadDLQMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) PurgeDLQMessages(ctx context.Context, request *adminv1.PurgeDLQMessagesRequest) (*adminv1.PurgeDLQMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) MergeDLQMessages(ctx context.Context, request *adminv1.MergeDLQMessagesRequest) (*adminv1.MergeDLQMessagesResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) RefreshWorkflowTasks(ctx context.Context, request *adminv1.RefreshWorkflowTasksRequest) (*adminv1.RefreshWorkflowTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) ResendReplicationTasks(ctx context.Context, request *adminv1.ResendReplicationTasksRequest) (*adminv1.ResendReplicationTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) GetCrossClusterTasks(ctx context.Context, request *adminv1.GetCrossClusterTasksRequest) (*adminv1.GetCrossClusterTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) RespondCrossClusterTasksCompleted(ctx context.Context, request *adminv1.RespondCrossClusterTasksCompletedRequest) (*adminv1.RespondCrossClusterTasksCompletedResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) GetDynamicConfig(ctx context.Context, request *adminv1.GetDynamicConfigRequest) (*adminv1.GetDynamicConfigResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) UpdateDynamicConfig(ctx context.Context, request *adminv1.UpdateDynamicConfigRequest) (*adminv1.UpdateDynamicConfigResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) RestoreDynamicConfig(ctx context.Context, request *adminv1.RestoreDynamicConfigRequest) (*adminv1.RestoreDynamicConfigResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) ListDynamicConfig(ctx context.Context, request *adminv1.ListDynamicConfigRequest) (*adminv1.ListDynamicConfigResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) DeleteWorkflow(ctx context.Context, request *adminv1.DeleteWorkflowRequest) (*adminv1.DeleteWorkflowResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) MaintainCorruptWorkflow(ctx context.Context, request *adminv1.MaintainCorruptWorkflowRequest) (*adminv1.MaintainCorruptWorkflowResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) GetGlobalIsolationGroups(ctx context.Context, request *adminv1.GetGlobalIsolationGroupsRequest) (*adminv1.GetGlobalIsolationGroupsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) UpdateGlobalIsolationGroups(ctx context.Context, request *adminv1.UpdateGlobalIsolationGroupsRequest) (*adminv1.UpdateGlobalIsolationGroupsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) GetDomainIsolationGroups(ctx context.Context, request *adminv1.GetDomainIsolationGroupsRequest) (*adminv1.GetDomainIsolationGroupsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) UpdateDomainIsolationGroups(ctx context.Context, request *adminv1.UpdateDomainIsolationGroupsRequest) (*adminv1.UpdateDomainIsolationGroupsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) GetDomainAsyncWorkflowConfiguraton(ctx context.Context, request *adminv1.GetDomainAsyncWorkflowConfiguratonRequest) (*adminv1.GetDomainAsyncWorkflowConfiguratonResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) UpdateDomainAsyncWorkflowConfiguraton(ctx context.Context, request *adminv1.UpdateDomainAsyncWorkflowConfiguratonRequest) (*adminv1.UpdateDomainAsyncWorkflowConfiguratonResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (a adminServiceProxyServer) UpdateTaskListPartitionConfig(ctx context.Context, request *adminv1.UpdateTaskListPartitionConfigRequest) (*adminv1.UpdateTaskListPartitionConfigResponse, error) {
	//TODO implement me
	panic("implement me")
}
