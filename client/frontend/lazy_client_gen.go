// Code generated by cmd/tools/genrpcwrappers. DO NOT EDIT.

package frontend

import (
	"context"

	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

func (c *lazyClient) CountWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.CountWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.CountWorkflowExecutionsResponse, error) {
	var resp *workflowservice.CountWorkflowExecutionsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.CountWorkflowExecutions(ctx, request, opts...)
}

func (c *lazyClient) CreateSchedule(
	ctx context.Context,
	request *workflowservice.CreateScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.CreateScheduleResponse, error) {
	var resp *workflowservice.CreateScheduleResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.CreateSchedule(ctx, request, opts...)
}

func (c *lazyClient) DeleteSchedule(
	ctx context.Context,
	request *workflowservice.DeleteScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DeleteScheduleResponse, error) {
	var resp *workflowservice.DeleteScheduleResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DeleteSchedule(ctx, request, opts...)
}

func (c *lazyClient) DeleteWorkflowExecution(
	ctx context.Context,
	request *workflowservice.DeleteWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DeleteWorkflowExecutionResponse, error) {
	var resp *workflowservice.DeleteWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DeleteWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) DeprecateNamespace(
	ctx context.Context,
	request *workflowservice.DeprecateNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DeprecateNamespaceResponse, error) {
	var resp *workflowservice.DeprecateNamespaceResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DeprecateNamespace(ctx, request, opts...)
}

func (c *lazyClient) DescribeBatchOperation(
	ctx context.Context,
	request *workflowservice.DescribeBatchOperationRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeBatchOperationResponse, error) {
	var resp *workflowservice.DescribeBatchOperationResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DescribeBatchOperation(ctx, request, opts...)
}

func (c *lazyClient) DescribeDeployment(
	ctx context.Context,
	request *workflowservice.DescribeDeploymentRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeDeploymentResponse, error) {
	var resp *workflowservice.DescribeDeploymentResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DescribeDeployment(ctx, request, opts...)
}

func (c *lazyClient) DescribeNamespace(
	ctx context.Context,
	request *workflowservice.DescribeNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeNamespaceResponse, error) {
	var resp *workflowservice.DescribeNamespaceResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DescribeNamespace(ctx, request, opts...)
}

func (c *lazyClient) DescribeSchedule(
	ctx context.Context,
	request *workflowservice.DescribeScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeScheduleResponse, error) {
	var resp *workflowservice.DescribeScheduleResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DescribeSchedule(ctx, request, opts...)
}

func (c *lazyClient) DescribeTaskQueue(
	ctx context.Context,
	request *workflowservice.DescribeTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeTaskQueueResponse, error) {
	var resp *workflowservice.DescribeTaskQueueResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DescribeTaskQueue(ctx, request, opts...)
}

func (c *lazyClient) DescribeWorkflowExecution(
	ctx context.Context,
	request *workflowservice.DescribeWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	var resp *workflowservice.DescribeWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.DescribeWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) ExecuteMultiOperation(
	ctx context.Context,
	request *workflowservice.ExecuteMultiOperationRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ExecuteMultiOperationResponse, error) {
	var resp *workflowservice.ExecuteMultiOperationResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ExecuteMultiOperation(ctx, request, opts...)
}

func (c *lazyClient) GetClusterInfo(
	ctx context.Context,
	request *workflowservice.GetClusterInfoRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetClusterInfoResponse, error) {
	var resp *workflowservice.GetClusterInfoResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetClusterInfo(ctx, request, opts...)
}

func (c *lazyClient) GetCurrentDeployment(
	ctx context.Context,
	request *workflowservice.GetCurrentDeploymentRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetCurrentDeploymentResponse, error) {
	var resp *workflowservice.GetCurrentDeploymentResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetCurrentDeployment(ctx, request, opts...)
}

func (c *lazyClient) GetDeploymentReachability(
	ctx context.Context,
	request *workflowservice.GetDeploymentReachabilityRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetDeploymentReachabilityResponse, error) {
	var resp *workflowservice.GetDeploymentReachabilityResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetDeploymentReachability(ctx, request, opts...)
}

func (c *lazyClient) GetSearchAttributes(
	ctx context.Context,
	request *workflowservice.GetSearchAttributesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetSearchAttributesResponse, error) {
	var resp *workflowservice.GetSearchAttributesResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetSearchAttributes(ctx, request, opts...)
}

func (c *lazyClient) GetSystemInfo(
	ctx context.Context,
	request *workflowservice.GetSystemInfoRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetSystemInfoResponse, error) {
	var resp *workflowservice.GetSystemInfoResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetSystemInfo(ctx, request, opts...)
}

func (c *lazyClient) GetWorkerBuildIdCompatibility(
	ctx context.Context,
	request *workflowservice.GetWorkerBuildIdCompatibilityRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkerBuildIdCompatibilityResponse, error) {
	var resp *workflowservice.GetWorkerBuildIdCompatibilityResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetWorkerBuildIdCompatibility(ctx, request, opts...)
}

func (c *lazyClient) GetWorkerTaskReachability(
	ctx context.Context,
	request *workflowservice.GetWorkerTaskReachabilityRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkerTaskReachabilityResponse, error) {
	var resp *workflowservice.GetWorkerTaskReachabilityResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetWorkerTaskReachability(ctx, request, opts...)
}

func (c *lazyClient) GetWorkerVersioningRules(
	ctx context.Context,
	request *workflowservice.GetWorkerVersioningRulesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkerVersioningRulesResponse, error) {
	var resp *workflowservice.GetWorkerVersioningRulesResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetWorkerVersioningRules(ctx, request, opts...)
}

func (c *lazyClient) GetWorkflowExecutionHistory(
	ctx context.Context,
	request *workflowservice.GetWorkflowExecutionHistoryRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	var resp *workflowservice.GetWorkflowExecutionHistoryResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetWorkflowExecutionHistory(ctx, request, opts...)
}

func (c *lazyClient) GetWorkflowExecutionHistoryReverse(
	ctx context.Context,
	request *workflowservice.GetWorkflowExecutionHistoryReverseRequest,
	opts ...grpc.CallOption,
) (*workflowservice.GetWorkflowExecutionHistoryReverseResponse, error) {
	var resp *workflowservice.GetWorkflowExecutionHistoryReverseResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.GetWorkflowExecutionHistoryReverse(ctx, request, opts...)
}

func (c *lazyClient) ListArchivedWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListArchivedWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListArchivedWorkflowExecutionsResponse, error) {
	var resp *workflowservice.ListArchivedWorkflowExecutionsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListArchivedWorkflowExecutions(ctx, request, opts...)
}

func (c *lazyClient) ListBatchOperations(
	ctx context.Context,
	request *workflowservice.ListBatchOperationsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListBatchOperationsResponse, error) {
	var resp *workflowservice.ListBatchOperationsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListBatchOperations(ctx, request, opts...)
}

func (c *lazyClient) ListClosedWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListClosedWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListClosedWorkflowExecutionsResponse, error) {
	var resp *workflowservice.ListClosedWorkflowExecutionsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListClosedWorkflowExecutions(ctx, request, opts...)
}

func (c *lazyClient) ListDeployments(
	ctx context.Context,
	request *workflowservice.ListDeploymentsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListDeploymentsResponse, error) {
	var resp *workflowservice.ListDeploymentsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListDeployments(ctx, request, opts...)
}

func (c *lazyClient) ListNamespaces(
	ctx context.Context,
	request *workflowservice.ListNamespacesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListNamespacesResponse, error) {
	var resp *workflowservice.ListNamespacesResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListNamespaces(ctx, request, opts...)
}

func (c *lazyClient) ListOpenWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListOpenWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListOpenWorkflowExecutionsResponse, error) {
	var resp *workflowservice.ListOpenWorkflowExecutionsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListOpenWorkflowExecutions(ctx, request, opts...)
}

func (c *lazyClient) ListScheduleMatchingTimes(
	ctx context.Context,
	request *workflowservice.ListScheduleMatchingTimesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListScheduleMatchingTimesResponse, error) {
	var resp *workflowservice.ListScheduleMatchingTimesResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListScheduleMatchingTimes(ctx, request, opts...)
}

func (c *lazyClient) ListSchedules(
	ctx context.Context,
	request *workflowservice.ListSchedulesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListSchedulesResponse, error) {
	var resp *workflowservice.ListSchedulesResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListSchedules(ctx, request, opts...)
}

func (c *lazyClient) ListTaskQueuePartitions(
	ctx context.Context,
	request *workflowservice.ListTaskQueuePartitionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListTaskQueuePartitionsResponse, error) {
	var resp *workflowservice.ListTaskQueuePartitionsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListTaskQueuePartitions(ctx, request, opts...)
}

func (c *lazyClient) ListWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ListWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	var resp *workflowservice.ListWorkflowExecutionsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ListWorkflowExecutions(ctx, request, opts...)
}

func (c *lazyClient) PatchSchedule(
	ctx context.Context,
	request *workflowservice.PatchScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PatchScheduleResponse, error) {
	var resp *workflowservice.PatchScheduleResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.PatchSchedule(ctx, request, opts...)
}

func (c *lazyClient) PauseActivityById(
	ctx context.Context,
	request *workflowservice.PauseActivityByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PauseActivityByIdResponse, error) {
	var resp *workflowservice.PauseActivityByIdResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.PauseActivityById(ctx, request, opts...)
}

func (c *lazyClient) PollActivityTaskQueue(
	ctx context.Context,
	request *workflowservice.PollActivityTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollActivityTaskQueueResponse, error) {
	var resp *workflowservice.PollActivityTaskQueueResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.PollActivityTaskQueue(ctx, request, opts...)
}

func (c *lazyClient) PollNexusTaskQueue(
	ctx context.Context,
	request *workflowservice.PollNexusTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollNexusTaskQueueResponse, error) {
	var resp *workflowservice.PollNexusTaskQueueResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.PollNexusTaskQueue(ctx, request, opts...)
}

func (c *lazyClient) PollWorkflowExecutionUpdate(
	ctx context.Context,
	request *workflowservice.PollWorkflowExecutionUpdateRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollWorkflowExecutionUpdateResponse, error) {
	var resp *workflowservice.PollWorkflowExecutionUpdateResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.PollWorkflowExecutionUpdate(ctx, request, opts...)
}

func (c *lazyClient) PollWorkflowTaskQueue(
	ctx context.Context,
	request *workflowservice.PollWorkflowTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.PollWorkflowTaskQueueResponse, error) {
	var resp *workflowservice.PollWorkflowTaskQueueResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.PollWorkflowTaskQueue(ctx, request, opts...)
}

func (c *lazyClient) QueryWorkflow(
	ctx context.Context,
	request *workflowservice.QueryWorkflowRequest,
	opts ...grpc.CallOption,
) (*workflowservice.QueryWorkflowResponse, error) {
	var resp *workflowservice.QueryWorkflowResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.QueryWorkflow(ctx, request, opts...)
}

func (c *lazyClient) RecordActivityTaskHeartbeat(
	ctx context.Context,
	request *workflowservice.RecordActivityTaskHeartbeatRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RecordActivityTaskHeartbeatResponse, error) {
	var resp *workflowservice.RecordActivityTaskHeartbeatResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RecordActivityTaskHeartbeat(ctx, request, opts...)
}

func (c *lazyClient) RecordActivityTaskHeartbeatById(
	ctx context.Context,
	request *workflowservice.RecordActivityTaskHeartbeatByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RecordActivityTaskHeartbeatByIdResponse, error) {
	var resp *workflowservice.RecordActivityTaskHeartbeatByIdResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RecordActivityTaskHeartbeatById(ctx, request, opts...)
}

func (c *lazyClient) RegisterNamespace(
	ctx context.Context,
	request *workflowservice.RegisterNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RegisterNamespaceResponse, error) {
	var resp *workflowservice.RegisterNamespaceResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RegisterNamespace(ctx, request, opts...)
}

func (c *lazyClient) RequestCancelWorkflowExecution(
	ctx context.Context,
	request *workflowservice.RequestCancelWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RequestCancelWorkflowExecutionResponse, error) {
	var resp *workflowservice.RequestCancelWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RequestCancelWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) ResetActivityById(
	ctx context.Context,
	request *workflowservice.ResetActivityByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ResetActivityByIdResponse, error) {
	var resp *workflowservice.ResetActivityByIdResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ResetActivityById(ctx, request, opts...)
}

func (c *lazyClient) ResetStickyTaskQueue(
	ctx context.Context,
	request *workflowservice.ResetStickyTaskQueueRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ResetStickyTaskQueueResponse, error) {
	var resp *workflowservice.ResetStickyTaskQueueResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ResetStickyTaskQueue(ctx, request, opts...)
}

func (c *lazyClient) ResetWorkflowExecution(
	ctx context.Context,
	request *workflowservice.ResetWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ResetWorkflowExecutionResponse, error) {
	var resp *workflowservice.ResetWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ResetWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) RespondActivityTaskCanceled(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCanceledRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCanceledResponse, error) {
	var resp *workflowservice.RespondActivityTaskCanceledResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondActivityTaskCanceled(ctx, request, opts...)
}

func (c *lazyClient) RespondActivityTaskCanceledById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCanceledByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCanceledByIdResponse, error) {
	var resp *workflowservice.RespondActivityTaskCanceledByIdResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondActivityTaskCanceledById(ctx, request, opts...)
}

func (c *lazyClient) RespondActivityTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCompletedResponse, error) {
	var resp *workflowservice.RespondActivityTaskCompletedResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondActivityTaskCompleted(ctx, request, opts...)
}

func (c *lazyClient) RespondActivityTaskCompletedById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskCompletedByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskCompletedByIdResponse, error) {
	var resp *workflowservice.RespondActivityTaskCompletedByIdResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondActivityTaskCompletedById(ctx, request, opts...)
}

func (c *lazyClient) RespondActivityTaskFailed(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskFailedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskFailedResponse, error) {
	var resp *workflowservice.RespondActivityTaskFailedResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondActivityTaskFailed(ctx, request, opts...)
}

func (c *lazyClient) RespondActivityTaskFailedById(
	ctx context.Context,
	request *workflowservice.RespondActivityTaskFailedByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondActivityTaskFailedByIdResponse, error) {
	var resp *workflowservice.RespondActivityTaskFailedByIdResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondActivityTaskFailedById(ctx, request, opts...)
}

func (c *lazyClient) RespondNexusTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondNexusTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondNexusTaskCompletedResponse, error) {
	var resp *workflowservice.RespondNexusTaskCompletedResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondNexusTaskCompleted(ctx, request, opts...)
}

func (c *lazyClient) RespondNexusTaskFailed(
	ctx context.Context,
	request *workflowservice.RespondNexusTaskFailedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondNexusTaskFailedResponse, error) {
	var resp *workflowservice.RespondNexusTaskFailedResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondNexusTaskFailed(ctx, request, opts...)
}

func (c *lazyClient) RespondQueryTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondQueryTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondQueryTaskCompletedResponse, error) {
	var resp *workflowservice.RespondQueryTaskCompletedResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondQueryTaskCompleted(ctx, request, opts...)
}

func (c *lazyClient) RespondWorkflowTaskCompleted(
	ctx context.Context,
	request *workflowservice.RespondWorkflowTaskCompletedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondWorkflowTaskCompletedResponse, error) {
	var resp *workflowservice.RespondWorkflowTaskCompletedResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondWorkflowTaskCompleted(ctx, request, opts...)
}

func (c *lazyClient) RespondWorkflowTaskFailed(
	ctx context.Context,
	request *workflowservice.RespondWorkflowTaskFailedRequest,
	opts ...grpc.CallOption,
) (*workflowservice.RespondWorkflowTaskFailedResponse, error) {
	var resp *workflowservice.RespondWorkflowTaskFailedResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.RespondWorkflowTaskFailed(ctx, request, opts...)
}

func (c *lazyClient) ScanWorkflowExecutions(
	ctx context.Context,
	request *workflowservice.ScanWorkflowExecutionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ScanWorkflowExecutionsResponse, error) {
	var resp *workflowservice.ScanWorkflowExecutionsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ScanWorkflowExecutions(ctx, request, opts...)
}

func (c *lazyClient) SetCurrentDeployment(
	ctx context.Context,
	request *workflowservice.SetCurrentDeploymentRequest,
	opts ...grpc.CallOption,
) (*workflowservice.SetCurrentDeploymentResponse, error) {
	var resp *workflowservice.SetCurrentDeploymentResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.SetCurrentDeployment(ctx, request, opts...)
}

func (c *lazyClient) ShutdownWorker(
	ctx context.Context,
	request *workflowservice.ShutdownWorkerRequest,
	opts ...grpc.CallOption,
) (*workflowservice.ShutdownWorkerResponse, error) {
	var resp *workflowservice.ShutdownWorkerResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.ShutdownWorker(ctx, request, opts...)
}

func (c *lazyClient) SignalWithStartWorkflowExecution(
	ctx context.Context,
	request *workflowservice.SignalWithStartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.SignalWithStartWorkflowExecutionResponse, error) {
	var resp *workflowservice.SignalWithStartWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.SignalWithStartWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) SignalWorkflowExecution(
	ctx context.Context,
	request *workflowservice.SignalWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.SignalWorkflowExecutionResponse, error) {
	var resp *workflowservice.SignalWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.SignalWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) StartBatchOperation(
	ctx context.Context,
	request *workflowservice.StartBatchOperationRequest,
	opts ...grpc.CallOption,
) (*workflowservice.StartBatchOperationResponse, error) {
	var resp *workflowservice.StartBatchOperationResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.StartBatchOperation(ctx, request, opts...)
}

func (c *lazyClient) StartWorkflowExecution(
	ctx context.Context,
	request *workflowservice.StartWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.StartWorkflowExecutionResponse, error) {
	var resp *workflowservice.StartWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.StartWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) StopBatchOperation(
	ctx context.Context,
	request *workflowservice.StopBatchOperationRequest,
	opts ...grpc.CallOption,
) (*workflowservice.StopBatchOperationResponse, error) {
	var resp *workflowservice.StopBatchOperationResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.StopBatchOperation(ctx, request, opts...)
}

func (c *lazyClient) TerminateWorkflowExecution(
	ctx context.Context,
	request *workflowservice.TerminateWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.TerminateWorkflowExecutionResponse, error) {
	var resp *workflowservice.TerminateWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.TerminateWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) UnpauseActivityById(
	ctx context.Context,
	request *workflowservice.UnpauseActivityByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UnpauseActivityByIdResponse, error) {
	var resp *workflowservice.UnpauseActivityByIdResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.UnpauseActivityById(ctx, request, opts...)
}

func (c *lazyClient) UpdateActivityOptionsById(
	ctx context.Context,
	request *workflowservice.UpdateActivityOptionsByIdRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateActivityOptionsByIdResponse, error) {
	var resp *workflowservice.UpdateActivityOptionsByIdResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.UpdateActivityOptionsById(ctx, request, opts...)
}

func (c *lazyClient) UpdateNamespace(
	ctx context.Context,
	request *workflowservice.UpdateNamespaceRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateNamespaceResponse, error) {
	var resp *workflowservice.UpdateNamespaceResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.UpdateNamespace(ctx, request, opts...)
}

func (c *lazyClient) UpdateSchedule(
	ctx context.Context,
	request *workflowservice.UpdateScheduleRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateScheduleResponse, error) {
	var resp *workflowservice.UpdateScheduleResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.UpdateSchedule(ctx, request, opts...)
}

func (c *lazyClient) UpdateWorkerBuildIdCompatibility(
	ctx context.Context,
	request *workflowservice.UpdateWorkerBuildIdCompatibilityRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateWorkerBuildIdCompatibilityResponse, error) {
	var resp *workflowservice.UpdateWorkerBuildIdCompatibilityResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.UpdateWorkerBuildIdCompatibility(ctx, request, opts...)
}

func (c *lazyClient) UpdateWorkerVersioningRules(
	ctx context.Context,
	request *workflowservice.UpdateWorkerVersioningRulesRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateWorkerVersioningRulesResponse, error) {
	var resp *workflowservice.UpdateWorkerVersioningRulesResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.UpdateWorkerVersioningRules(ctx, request, opts...)
}

func (c *lazyClient) UpdateWorkflowExecution(
	ctx context.Context,
	request *workflowservice.UpdateWorkflowExecutionRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateWorkflowExecutionResponse, error) {
	var resp *workflowservice.UpdateWorkflowExecutionResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.UpdateWorkflowExecution(ctx, request, opts...)
}

func (c *lazyClient) UpdateWorkflowExecutionOptions(
	ctx context.Context,
	request *workflowservice.UpdateWorkflowExecutionOptionsRequest,
	opts ...grpc.CallOption,
) (*workflowservice.UpdateWorkflowExecutionOptionsResponse, error) {
	var resp *workflowservice.UpdateWorkflowExecutionOptionsResponse
	client, err := c.clientProvider.GetWorkflowServiceClient()
	if err != nil {
		return resp, err
	}
	return client.UpdateWorkflowExecutionOptions(ctx, request, opts...)
}
