package proxy

import (
	"context"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
)

type (
	workflowServiceProxyServer struct {
		workflowservice.UnimplementedWorkflowServiceServer
		clientProvider client.ClientProvider
		logger         log.Logger
	}
)

// NewWorkflowServiceProxyServer creates a WorkflowServiceServer suitable for registering with a gRPC Server. Requests will
// be forwarded to the passed in WorkflowService Client. gRPC interceptors can be added on the Server or Client to adjust
// requests and responses.
func NewWorkflowServiceProxyServer(
	serviceName string,
	clientConfig config.ClientConfig,
	clientFactory client.ClientFactory,
	logger log.Logger,
) workflowservice.WorkflowServiceServer {
	logger = log.With(logger, common.ServiceTag(serviceName))
	return &workflowServiceProxyServer{
		clientProvider: client.NewClientProvider(clientConfig, clientFactory, logger),
		logger:         logger,
	}
}

func (s *workflowServiceProxyServer) CountWorkflowExecutions(ctx context.Context, in0 *workflowservice.CountWorkflowExecutionsRequest) (*workflowservice.CountWorkflowExecutionsResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.CountWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) CreateSchedule(ctx context.Context, in0 *workflowservice.CreateScheduleRequest) (*workflowservice.CreateScheduleResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.CreateSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) DeleteSchedule(ctx context.Context, in0 *workflowservice.DeleteScheduleRequest) (*workflowservice.DeleteScheduleResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.DeleteSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) DeleteWorkflowExecution(ctx context.Context, in0 *workflowservice.DeleteWorkflowExecutionRequest) (*workflowservice.DeleteWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.DeleteWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) DeprecateNamespace(ctx context.Context, in0 *workflowservice.DeprecateNamespaceRequest) (*workflowservice.DeprecateNamespaceResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.DeprecateNamespace(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeBatchOperation(ctx context.Context, in0 *workflowservice.DescribeBatchOperationRequest) (*workflowservice.DescribeBatchOperationResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.DescribeBatchOperation(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeNamespace(ctx context.Context, in0 *workflowservice.DescribeNamespaceRequest) (*workflowservice.DescribeNamespaceResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.DescribeNamespace(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeSchedule(ctx context.Context, in0 *workflowservice.DescribeScheduleRequest) (*workflowservice.DescribeScheduleResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.DescribeSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeTaskQueue(ctx context.Context, in0 *workflowservice.DescribeTaskQueueRequest) (*workflowservice.DescribeTaskQueueResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.DescribeTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) DescribeWorkflowExecution(ctx context.Context, in0 *workflowservice.DescribeWorkflowExecutionRequest) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.DescribeWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) ExecuteMultiOperation(ctx context.Context, in0 *workflowservice.ExecuteMultiOperationRequest) (*workflowservice.ExecuteMultiOperationResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ExecuteMultiOperation(ctx, in0)
}

func (s *workflowServiceProxyServer) GetClusterInfo(ctx context.Context, in0 *workflowservice.GetClusterInfoRequest) (*workflowservice.GetClusterInfoResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.GetClusterInfo(ctx, in0)
}

func (s *workflowServiceProxyServer) GetSearchAttributes(ctx context.Context, in0 *workflowservice.GetSearchAttributesRequest) (*workflowservice.GetSearchAttributesResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.GetSearchAttributes(ctx, in0)
}

func (s *workflowServiceProxyServer) GetSystemInfo(ctx context.Context, in0 *workflowservice.GetSystemInfoRequest) (*workflowservice.GetSystemInfoResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.GetSystemInfo(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkerBuildIdCompatibility(ctx context.Context, in0 *workflowservice.GetWorkerBuildIdCompatibilityRequest) (*workflowservice.GetWorkerBuildIdCompatibilityResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.GetWorkerBuildIdCompatibility(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkerTaskReachability(ctx context.Context, in0 *workflowservice.GetWorkerTaskReachabilityRequest) (*workflowservice.GetWorkerTaskReachabilityResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.GetWorkerTaskReachability(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkerVersioningRules(ctx context.Context, in0 *workflowservice.GetWorkerVersioningRulesRequest) (*workflowservice.GetWorkerVersioningRulesResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.GetWorkerVersioningRules(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkflowExecutionHistory(ctx context.Context, in0 *workflowservice.GetWorkflowExecutionHistoryRequest) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.GetWorkflowExecutionHistory(ctx, in0)
}

func (s *workflowServiceProxyServer) GetWorkflowExecutionHistoryReverse(ctx context.Context, in0 *workflowservice.GetWorkflowExecutionHistoryReverseRequest) (*workflowservice.GetWorkflowExecutionHistoryReverseResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.GetWorkflowExecutionHistoryReverse(ctx, in0)
}

func (s *workflowServiceProxyServer) ListArchivedWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListArchivedWorkflowExecutionsRequest) (*workflowservice.ListArchivedWorkflowExecutionsResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListArchivedWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) ListBatchOperations(ctx context.Context, in0 *workflowservice.ListBatchOperationsRequest) (*workflowservice.ListBatchOperationsResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListBatchOperations(ctx, in0)
}

func (s *workflowServiceProxyServer) ListClosedWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListClosedWorkflowExecutionsRequest) (*workflowservice.ListClosedWorkflowExecutionsResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListClosedWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) ListNamespaces(ctx context.Context, in0 *workflowservice.ListNamespacesRequest) (*workflowservice.ListNamespacesResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListNamespaces(ctx, in0)
}

func (s *workflowServiceProxyServer) ListOpenWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListOpenWorkflowExecutionsRequest) (*workflowservice.ListOpenWorkflowExecutionsResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListOpenWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) ListScheduleMatchingTimes(ctx context.Context, in0 *workflowservice.ListScheduleMatchingTimesRequest) (*workflowservice.ListScheduleMatchingTimesResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListScheduleMatchingTimes(ctx, in0)
}

func (s *workflowServiceProxyServer) ListSchedules(ctx context.Context, in0 *workflowservice.ListSchedulesRequest) (*workflowservice.ListSchedulesResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListSchedules(ctx, in0)
}

func (s *workflowServiceProxyServer) ListTaskQueuePartitions(ctx context.Context, in0 *workflowservice.ListTaskQueuePartitionsRequest) (*workflowservice.ListTaskQueuePartitionsResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListTaskQueuePartitions(ctx, in0)
}

func (s *workflowServiceProxyServer) ListWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListWorkflowExecutionsRequest) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ListWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) PatchSchedule(ctx context.Context, in0 *workflowservice.PatchScheduleRequest) (*workflowservice.PatchScheduleResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.PatchSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) PollActivityTaskQueue(ctx context.Context, in0 *workflowservice.PollActivityTaskQueueRequest) (*workflowservice.PollActivityTaskQueueResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.PollActivityTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) PollNexusTaskQueue(ctx context.Context, in0 *workflowservice.PollNexusTaskQueueRequest) (*workflowservice.PollNexusTaskQueueResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.PollNexusTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) PollWorkflowExecutionUpdate(ctx context.Context, in0 *workflowservice.PollWorkflowExecutionUpdateRequest) (*workflowservice.PollWorkflowExecutionUpdateResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.PollWorkflowExecutionUpdate(ctx, in0)
}

func (s *workflowServiceProxyServer) PollWorkflowTaskQueue(ctx context.Context, in0 *workflowservice.PollWorkflowTaskQueueRequest) (*workflowservice.PollWorkflowTaskQueueResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.PollWorkflowTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) QueryWorkflow(ctx context.Context, in0 *workflowservice.QueryWorkflowRequest) (*workflowservice.QueryWorkflowResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.QueryWorkflow(ctx, in0)
}

func (s *workflowServiceProxyServer) RecordActivityTaskHeartbeat(ctx context.Context, in0 *workflowservice.RecordActivityTaskHeartbeatRequest) (*workflowservice.RecordActivityTaskHeartbeatResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RecordActivityTaskHeartbeat(ctx, in0)
}

func (s *workflowServiceProxyServer) RecordActivityTaskHeartbeatById(ctx context.Context, in0 *workflowservice.RecordActivityTaskHeartbeatByIdRequest) (*workflowservice.RecordActivityTaskHeartbeatByIdResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RecordActivityTaskHeartbeatById(ctx, in0)
}

func (s *workflowServiceProxyServer) RegisterNamespace(ctx context.Context, in0 *workflowservice.RegisterNamespaceRequest) (*workflowservice.RegisterNamespaceResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RegisterNamespace(ctx, in0)
}

func (s *workflowServiceProxyServer) RequestCancelWorkflowExecution(ctx context.Context, in0 *workflowservice.RequestCancelWorkflowExecutionRequest) (*workflowservice.RequestCancelWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RequestCancelWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) ResetStickyTaskQueue(ctx context.Context, in0 *workflowservice.ResetStickyTaskQueueRequest) (*workflowservice.ResetStickyTaskQueueResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ResetStickyTaskQueue(ctx, in0)
}

func (s *workflowServiceProxyServer) ResetWorkflowExecution(ctx context.Context, in0 *workflowservice.ResetWorkflowExecutionRequest) (*workflowservice.ResetWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ResetWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCanceled(ctx context.Context, in0 *workflowservice.RespondActivityTaskCanceledRequest) (*workflowservice.RespondActivityTaskCanceledResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondActivityTaskCanceled(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCanceledById(ctx context.Context, in0 *workflowservice.RespondActivityTaskCanceledByIdRequest) (*workflowservice.RespondActivityTaskCanceledByIdResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondActivityTaskCanceledById(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCompleted(ctx context.Context, in0 *workflowservice.RespondActivityTaskCompletedRequest) (*workflowservice.RespondActivityTaskCompletedResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondActivityTaskCompleted(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCompletedById(ctx context.Context, in0 *workflowservice.RespondActivityTaskCompletedByIdRequest) (*workflowservice.RespondActivityTaskCompletedByIdResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondActivityTaskCompletedById(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskFailed(ctx context.Context, in0 *workflowservice.RespondActivityTaskFailedRequest) (*workflowservice.RespondActivityTaskFailedResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondActivityTaskFailed(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskFailedById(ctx context.Context, in0 *workflowservice.RespondActivityTaskFailedByIdRequest) (*workflowservice.RespondActivityTaskFailedByIdResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondActivityTaskFailedById(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondNexusTaskCompleted(ctx context.Context, in0 *workflowservice.RespondNexusTaskCompletedRequest) (*workflowservice.RespondNexusTaskCompletedResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondNexusTaskCompleted(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondNexusTaskFailed(ctx context.Context, in0 *workflowservice.RespondNexusTaskFailedRequest) (*workflowservice.RespondNexusTaskFailedResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondNexusTaskFailed(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondQueryTaskCompleted(ctx context.Context, in0 *workflowservice.RespondQueryTaskCompletedRequest) (*workflowservice.RespondQueryTaskCompletedResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondQueryTaskCompleted(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondWorkflowTaskCompleted(ctx context.Context, in0 *workflowservice.RespondWorkflowTaskCompletedRequest) (*workflowservice.RespondWorkflowTaskCompletedResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondWorkflowTaskCompleted(ctx, in0)
}

func (s *workflowServiceProxyServer) RespondWorkflowTaskFailed(ctx context.Context, in0 *workflowservice.RespondWorkflowTaskFailedRequest) (*workflowservice.RespondWorkflowTaskFailedResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.RespondWorkflowTaskFailed(ctx, in0)
}

func (s *workflowServiceProxyServer) ScanWorkflowExecutions(ctx context.Context, in0 *workflowservice.ScanWorkflowExecutionsRequest) (*workflowservice.ScanWorkflowExecutionsResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.ScanWorkflowExecutions(ctx, in0)
}

func (s *workflowServiceProxyServer) SignalWithStartWorkflowExecution(ctx context.Context, in0 *workflowservice.SignalWithStartWorkflowExecutionRequest) (*workflowservice.SignalWithStartWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.SignalWithStartWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) SignalWorkflowExecution(ctx context.Context, in0 *workflowservice.SignalWorkflowExecutionRequest) (*workflowservice.SignalWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.SignalWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) StartBatchOperation(ctx context.Context, in0 *workflowservice.StartBatchOperationRequest) (*workflowservice.StartBatchOperationResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.StartBatchOperation(ctx, in0)
}

func (s *workflowServiceProxyServer) StartWorkflowExecution(ctx context.Context, in0 *workflowservice.StartWorkflowExecutionRequest) (*workflowservice.StartWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.StartWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) StopBatchOperation(ctx context.Context, in0 *workflowservice.StopBatchOperationRequest) (*workflowservice.StopBatchOperationResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.StopBatchOperation(ctx, in0)
}

func (s *workflowServiceProxyServer) TerminateWorkflowExecution(ctx context.Context, in0 *workflowservice.TerminateWorkflowExecutionRequest) (*workflowservice.TerminateWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.TerminateWorkflowExecution(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateNamespace(ctx context.Context, in0 *workflowservice.UpdateNamespaceRequest) (*workflowservice.UpdateNamespaceResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.UpdateNamespace(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateSchedule(ctx context.Context, in0 *workflowservice.UpdateScheduleRequest) (*workflowservice.UpdateScheduleResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.UpdateSchedule(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerBuildIdCompatibility(ctx context.Context, in0 *workflowservice.UpdateWorkerBuildIdCompatibilityRequest) (*workflowservice.UpdateWorkerBuildIdCompatibilityResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.UpdateWorkerBuildIdCompatibility(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerVersioningRules(ctx context.Context, in0 *workflowservice.UpdateWorkerVersioningRulesRequest) (*workflowservice.UpdateWorkerVersioningRulesResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.UpdateWorkerVersioningRules(ctx, in0)
}

func (s *workflowServiceProxyServer) UpdateWorkflowExecution(ctx context.Context, in0 *workflowservice.UpdateWorkflowExecutionRequest) (*workflowservice.UpdateWorkflowExecutionResponse, error) {
	wfclient, err := s.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return nil, err
	}
	return wfclient.UpdateWorkflowExecution(ctx, in0)
}