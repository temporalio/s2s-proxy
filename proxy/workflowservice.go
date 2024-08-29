package proxy

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

type (
	workflowServiceProxyServer struct {
		workflowservice.UnimplementedWorkflowServiceServer
		proxyConfig                 config.ProxyConfig
		logger                      log.Logger
		remoteWorkflowServiceClient workflowservice.WorkflowServiceClient
	}
)

// NewWorkflowServiceProxyServer creates a WorkflowServiceServer suitable for registering with a gRPC Server. Requests will
// be forwarded to the passed in WorkflowService Client. gRPC interceptors can be added on the Server or Client to adjust
// requests and responses.
func NewWorkflowServiceProxyServer(
	proxyConfig config.ProxyConfig,
	clientFactory client.ClientFactory,
	logger log.Logger,
) workflowservice.WorkflowServiceServer {
	logger = log.With(logger,
		common.ServiceTag(proxyConfig.Name),
		tag.Address(proxyConfig.Server.ListenAddress),
	)
	if data, err := json.Marshal(proxyConfig.Client); err == nil {
		logger.Info(fmt.Sprintf("RemoteWorkflowClient Config: %s", string(data)))
	} else {
		logger.Error(fmt.Sprintf("RemoteWorkflowClient: failed to marshal config: %v", err))
		return nil
	}

	// TODO: Switch to use ClientProvider to late bind the client.
	worklowclient, err := clientFactory.NewRemoteWorkflowServiceClient(proxyConfig.Client)
	if err != nil {
		logger.Fatal("Failed to create workflowclient")
	}

	return &workflowServiceProxyServer{
		proxyConfig:                 proxyConfig,
		logger:                      logger,
		remoteWorkflowServiceClient: worklowclient,
	}
}

func (s *workflowServiceProxyServer) CountWorkflowExecutions(ctx context.Context, in0 *workflowservice.CountWorkflowExecutionsRequest) (*workflowservice.CountWorkflowExecutionsResponse, error) {
	return s.remoteWorkflowServiceClient.CountWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) CreateSchedule(ctx context.Context, in0 *workflowservice.CreateScheduleRequest) (*workflowservice.CreateScheduleResponse, error) {
	return s.remoteWorkflowServiceClient.CreateSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) DeleteSchedule(ctx context.Context, in0 *workflowservice.DeleteScheduleRequest) (*workflowservice.DeleteScheduleResponse, error) {
	return s.remoteWorkflowServiceClient.DeleteSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) DeleteWorkflowExecution(ctx context.Context, in0 *workflowservice.DeleteWorkflowExecutionRequest) (*workflowservice.DeleteWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.DeleteWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) DeprecateNamespace(ctx context.Context, in0 *workflowservice.DeprecateNamespaceRequest) (*workflowservice.DeprecateNamespaceResponse, error) {
	return s.remoteWorkflowServiceClient.DeprecateNamespace(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeBatchOperation(ctx context.Context, in0 *workflowservice.DescribeBatchOperationRequest) (*workflowservice.DescribeBatchOperationResponse, error) {
	return s.remoteWorkflowServiceClient.DescribeBatchOperation(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeNamespace(ctx context.Context, in0 *workflowservice.DescribeNamespaceRequest) (*workflowservice.DescribeNamespaceResponse, error) {
	return s.remoteWorkflowServiceClient.DescribeNamespace(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeSchedule(ctx context.Context, in0 *workflowservice.DescribeScheduleRequest) (*workflowservice.DescribeScheduleResponse, error) {
	return s.remoteWorkflowServiceClient.DescribeSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeTaskQueue(ctx context.Context, in0 *workflowservice.DescribeTaskQueueRequest) (*workflowservice.DescribeTaskQueueResponse, error) {
	return s.remoteWorkflowServiceClient.DescribeTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeWorkflowExecution(ctx context.Context, in0 *workflowservice.DescribeWorkflowExecutionRequest) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.DescribeWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) ExecuteMultiOperation(ctx context.Context, in0 *workflowservice.ExecuteMultiOperationRequest) (*workflowservice.ExecuteMultiOperationResponse, error) {
	return s.remoteWorkflowServiceClient.ExecuteMultiOperation(ctx, in0)
}

func (s *workflowServiceProxyServer) GetClusterInfo(ctx context.Context, in0 *workflowservice.GetClusterInfoRequest) (*workflowservice.GetClusterInfoResponse, error) {
	return s.remoteWorkflowServiceClient.GetClusterInfo(ctx, in0)
}

func (s *workflowServiceProxyServer) GetSearchAttributes(ctx context.Context, in0 *workflowservice.GetSearchAttributesRequest) (*workflowservice.GetSearchAttributesResponse, error) {
	return s.remoteWorkflowServiceClient.GetSearchAttributes(ctx, in0)
}

func (s *workflowServiceProxyServer) GetSystemInfo(ctx context.Context, in0 *workflowservice.GetSystemInfoRequest) (*workflowservice.GetSystemInfoResponse, error) {
	return s.remoteWorkflowServiceClient.GetSystemInfo(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkerBuildIdCompatibility(ctx context.Context, in0 *workflowservice.GetWorkerBuildIdCompatibilityRequest) (*workflowservice.GetWorkerBuildIdCompatibilityResponse, error) {
	return s.remoteWorkflowServiceClient.GetWorkerBuildIdCompatibility(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkerTaskReachability(ctx context.Context, in0 *workflowservice.GetWorkerTaskReachabilityRequest) (*workflowservice.GetWorkerTaskReachabilityResponse, error) {
	return s.remoteWorkflowServiceClient.GetWorkerTaskReachability(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkerVersioningRules(ctx context.Context, in0 *workflowservice.GetWorkerVersioningRulesRequest) (*workflowservice.GetWorkerVersioningRulesResponse, error) {
	return s.remoteWorkflowServiceClient.GetWorkerVersioningRules(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkflowExecutionHistory(ctx context.Context, in0 *workflowservice.GetWorkflowExecutionHistoryRequest) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	return s.remoteWorkflowServiceClient.GetWorkflowExecutionHistory(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkflowExecutionHistoryReverse(ctx context.Context, in0 *workflowservice.GetWorkflowExecutionHistoryReverseRequest) (*workflowservice.GetWorkflowExecutionHistoryReverseResponse, error) {
	return s.remoteWorkflowServiceClient.GetWorkflowExecutionHistoryReverse(ctx, in0)
}

func (s *workflowServiceProxyServer) ListArchivedWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListArchivedWorkflowExecutionsRequest) (*workflowservice.ListArchivedWorkflowExecutionsResponse, error) {
	return s.remoteWorkflowServiceClient.ListArchivedWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) ListBatchOperations(ctx context.Context, in0 *workflowservice.ListBatchOperationsRequest) (*workflowservice.ListBatchOperationsResponse, error) {
	return s.remoteWorkflowServiceClient.ListBatchOperations(ctx, in0)
}

func (s *workflowServiceProxyServer) ListClosedWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListClosedWorkflowExecutionsRequest) (*workflowservice.ListClosedWorkflowExecutionsResponse, error) {
	return s.remoteWorkflowServiceClient.ListClosedWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) ListNamespaces(ctx context.Context, in0 *workflowservice.ListNamespacesRequest) (*workflowservice.ListNamespacesResponse, error) {
	return s.remoteWorkflowServiceClient.ListNamespaces(ctx, in0)
}

func (s *workflowServiceProxyServer) ListOpenWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListOpenWorkflowExecutionsRequest) (*workflowservice.ListOpenWorkflowExecutionsResponse, error) {
	return s.remoteWorkflowServiceClient.ListOpenWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) ListScheduleMatchingTimes(ctx context.Context, in0 *workflowservice.ListScheduleMatchingTimesRequest) (*workflowservice.ListScheduleMatchingTimesResponse, error) {
	return s.remoteWorkflowServiceClient.ListScheduleMatchingTimes(ctx, in0)
}

func (s *workflowServiceProxyServer) ListSchedules(ctx context.Context, in0 *workflowservice.ListSchedulesRequest) (*workflowservice.ListSchedulesResponse, error) {
	return s.remoteWorkflowServiceClient.ListSchedules(ctx, in0)
}

func (s *workflowServiceProxyServer) ListTaskQueuePartitions(ctx context.Context, in0 *workflowservice.ListTaskQueuePartitionsRequest) (*workflowservice.ListTaskQueuePartitionsResponse, error) {
	return s.remoteWorkflowServiceClient.ListTaskQueuePartitions(ctx, in0)
}

func (s *workflowServiceProxyServer) ListWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListWorkflowExecutionsRequest) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	return s.remoteWorkflowServiceClient.ListWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) PatchSchedule(ctx context.Context, in0 *workflowservice.PatchScheduleRequest) (*workflowservice.PatchScheduleResponse, error) {
	return s.remoteWorkflowServiceClient.PatchSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) PollActivityTaskQueue(ctx context.Context, in0 *workflowservice.PollActivityTaskQueueRequest) (*workflowservice.PollActivityTaskQueueResponse, error) {
	return s.remoteWorkflowServiceClient.PollActivityTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) PollNexusTaskQueue(ctx context.Context, in0 *workflowservice.PollNexusTaskQueueRequest) (*workflowservice.PollNexusTaskQueueResponse, error) {
	return s.remoteWorkflowServiceClient.PollNexusTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) PollWorkflowExecutionUpdate(ctx context.Context, in0 *workflowservice.PollWorkflowExecutionUpdateRequest) (*workflowservice.PollWorkflowExecutionUpdateResponse, error) {
	return s.remoteWorkflowServiceClient.PollWorkflowExecutionUpdate(ctx, in0)
}

func (s *workflowServiceProxyServer) PollWorkflowTaskQueue(ctx context.Context, in0 *workflowservice.PollWorkflowTaskQueueRequest) (*workflowservice.PollWorkflowTaskQueueResponse, error) {
	return s.remoteWorkflowServiceClient.PollWorkflowTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) QueryWorkflow(ctx context.Context, in0 *workflowservice.QueryWorkflowRequest) (*workflowservice.QueryWorkflowResponse, error) {
	return s.remoteWorkflowServiceClient.QueryWorkflow(ctx, in0)
}

func (s *workflowServiceProxyServer) RecordActivityTaskHeartbeat(ctx context.Context, in0 *workflowservice.RecordActivityTaskHeartbeatRequest) (*workflowservice.RecordActivityTaskHeartbeatResponse, error) {
	return s.remoteWorkflowServiceClient.RecordActivityTaskHeartbeat(ctx, in0)
}

func (s *workflowServiceProxyServer) RecordActivityTaskHeartbeatById(ctx context.Context, in0 *workflowservice.RecordActivityTaskHeartbeatByIdRequest) (*workflowservice.RecordActivityTaskHeartbeatByIdResponse, error) {
	return s.remoteWorkflowServiceClient.RecordActivityTaskHeartbeatById(ctx, in0)
}

func (s *workflowServiceProxyServer) RegisterNamespace(ctx context.Context, in0 *workflowservice.RegisterNamespaceRequest) (*workflowservice.RegisterNamespaceResponse, error) {
	return s.remoteWorkflowServiceClient.RegisterNamespace(ctx, in0)
}

func (s *workflowServiceProxyServer) RequestCancelWorkflowExecution(ctx context.Context, in0 *workflowservice.RequestCancelWorkflowExecutionRequest) (*workflowservice.RequestCancelWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.RequestCancelWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) ResetStickyTaskQueue(ctx context.Context, in0 *workflowservice.ResetStickyTaskQueueRequest) (*workflowservice.ResetStickyTaskQueueResponse, error) {
	return s.remoteWorkflowServiceClient.ResetStickyTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) ResetWorkflowExecution(ctx context.Context, in0 *workflowservice.ResetWorkflowExecutionRequest) (*workflowservice.ResetWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.ResetWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCanceled(ctx context.Context, in0 *workflowservice.RespondActivityTaskCanceledRequest) (*workflowservice.RespondActivityTaskCanceledResponse, error) {
	return s.remoteWorkflowServiceClient.RespondActivityTaskCanceled(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCanceledById(ctx context.Context, in0 *workflowservice.RespondActivityTaskCanceledByIdRequest) (*workflowservice.RespondActivityTaskCanceledByIdResponse, error) {
	return s.remoteWorkflowServiceClient.RespondActivityTaskCanceledById(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCompleted(ctx context.Context, in0 *workflowservice.RespondActivityTaskCompletedRequest) (*workflowservice.RespondActivityTaskCompletedResponse, error) {
	return s.remoteWorkflowServiceClient.RespondActivityTaskCompleted(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCompletedById(ctx context.Context, in0 *workflowservice.RespondActivityTaskCompletedByIdRequest) (*workflowservice.RespondActivityTaskCompletedByIdResponse, error) {
	return s.remoteWorkflowServiceClient.RespondActivityTaskCompletedById(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskFailed(ctx context.Context, in0 *workflowservice.RespondActivityTaskFailedRequest) (*workflowservice.RespondActivityTaskFailedResponse, error) {
	return s.remoteWorkflowServiceClient.RespondActivityTaskFailed(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskFailedById(ctx context.Context, in0 *workflowservice.RespondActivityTaskFailedByIdRequest) (*workflowservice.RespondActivityTaskFailedByIdResponse, error) {
	return s.remoteWorkflowServiceClient.RespondActivityTaskFailedById(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondNexusTaskCompleted(ctx context.Context, in0 *workflowservice.RespondNexusTaskCompletedRequest) (*workflowservice.RespondNexusTaskCompletedResponse, error) {
	return s.remoteWorkflowServiceClient.RespondNexusTaskCompleted(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondNexusTaskFailed(ctx context.Context, in0 *workflowservice.RespondNexusTaskFailedRequest) (*workflowservice.RespondNexusTaskFailedResponse, error) {
	return s.remoteWorkflowServiceClient.RespondNexusTaskFailed(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondQueryTaskCompleted(ctx context.Context, in0 *workflowservice.RespondQueryTaskCompletedRequest) (*workflowservice.RespondQueryTaskCompletedResponse, error) {
	return s.remoteWorkflowServiceClient.RespondQueryTaskCompleted(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondWorkflowTaskCompleted(ctx context.Context, in0 *workflowservice.RespondWorkflowTaskCompletedRequest) (*workflowservice.RespondWorkflowTaskCompletedResponse, error) {
	return s.remoteWorkflowServiceClient.RespondWorkflowTaskCompleted(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondWorkflowTaskFailed(ctx context.Context, in0 *workflowservice.RespondWorkflowTaskFailedRequest) (*workflowservice.RespondWorkflowTaskFailedResponse, error) {
	return s.remoteWorkflowServiceClient.RespondWorkflowTaskFailed(ctx, in0)
}

func (s *workflowServiceProxyServer) ScanWorkflowExecutions(ctx context.Context, in0 *workflowservice.ScanWorkflowExecutionsRequest) (*workflowservice.ScanWorkflowExecutionsResponse, error) {
	return s.remoteWorkflowServiceClient.ScanWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) SignalWithStartWorkflowExecution(ctx context.Context, in0 *workflowservice.SignalWithStartWorkflowExecutionRequest) (*workflowservice.SignalWithStartWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.SignalWithStartWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) SignalWorkflowExecution(ctx context.Context, in0 *workflowservice.SignalWorkflowExecutionRequest) (*workflowservice.SignalWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.SignalWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) StartBatchOperation(ctx context.Context, in0 *workflowservice.StartBatchOperationRequest) (*workflowservice.StartBatchOperationResponse, error) {
	return s.remoteWorkflowServiceClient.StartBatchOperation(ctx, in0)
}

func (s *workflowServiceProxyServer) StartWorkflowExecution(ctx context.Context, in0 *workflowservice.StartWorkflowExecutionRequest) (*workflowservice.StartWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.StartWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) StopBatchOperation(ctx context.Context, in0 *workflowservice.StopBatchOperationRequest) (*workflowservice.StopBatchOperationResponse, error) {
	return s.remoteWorkflowServiceClient.StopBatchOperation(ctx, in0)
}

func (s *workflowServiceProxyServer) TerminateWorkflowExecution(ctx context.Context, in0 *workflowservice.TerminateWorkflowExecutionRequest) (*workflowservice.TerminateWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.TerminateWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateNamespace(ctx context.Context, in0 *workflowservice.UpdateNamespaceRequest) (*workflowservice.UpdateNamespaceResponse, error) {
	return s.remoteWorkflowServiceClient.UpdateNamespace(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateSchedule(ctx context.Context, in0 *workflowservice.UpdateScheduleRequest) (*workflowservice.UpdateScheduleResponse, error) {
	return s.remoteWorkflowServiceClient.UpdateSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerBuildIdCompatibility(ctx context.Context, in0 *workflowservice.UpdateWorkerBuildIdCompatibilityRequest) (*workflowservice.UpdateWorkerBuildIdCompatibilityResponse, error) {
	return s.remoteWorkflowServiceClient.UpdateWorkerBuildIdCompatibility(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerVersioningRules(ctx context.Context, in0 *workflowservice.UpdateWorkerVersioningRulesRequest) (*workflowservice.UpdateWorkerVersioningRulesResponse, error) {
	return s.remoteWorkflowServiceClient.UpdateWorkerVersioningRules(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateWorkflowExecution(ctx context.Context, in0 *workflowservice.UpdateWorkflowExecutionRequest) (*workflowservice.UpdateWorkflowExecutionResponse, error) {
	return s.remoteWorkflowServiceClient.UpdateWorkflowExecution(ctx, in0)
}
