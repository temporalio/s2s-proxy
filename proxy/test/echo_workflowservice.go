package proxy

import (
	"context"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

type (
	echoWorkflowService struct {
		workflowservice.UnimplementedWorkflowServiceServer
		serviceName string
		logger      log.Logger
	}
)

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
