package proxy

import (
	"context"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/common"
)

const DCRedirectionContextHeaderName = "xdc-redirection" // https://github.com/temporalio/temporal/blob/9a1060c4162ff62576cb899d7e5b1bae179af814/common/rpc/interceptor/redirection.go#L27

type (
	workflowServiceProxyServer struct {
		workflowservice.UnimplementedWorkflowServiceServer
		workflowServiceClient workflowservice.WorkflowServiceClient
		namespaceAccess       *auth.AccessControl
		logger                log.Logger
	}
)

// NewWorkflowServiceProxyServer creates a WorkflowServiceServer suitable for registering with a gRPC Server. Requests will
// be forwarded to the passed in WorkflowService Client. gRPC interceptors can be added on the Server or Client to adjust
// requests and responses.
func NewWorkflowServiceProxyServer(
	serviceName string,
	workflowServiceClient workflowservice.WorkflowServiceClient,
	namespaceAccess *auth.AccessControl,
	logger log.Logger,
) workflowservice.WorkflowServiceServer {
	logger = log.With(logger, common.ServiceTag(serviceName))
	return &workflowServiceProxyServer{
		workflowServiceClient: workflowServiceClient,
		namespaceAccess:       namespaceAccess,
		logger:                logger,
	}
}

// ListNamespaces wraps the same method on the underlying workflowservice.WorkflowServiceClient.
// In particular, this version checks the returned namespaces against the configured ACL and makes sure we're not
// returning disallowed namespaces to the customer.
func (s *workflowServiceProxyServer) ListNamespaces(ctx context.Context, req *workflowservice.ListNamespacesRequest) (*workflowservice.ListNamespacesResponse, error) {
	response, err := s.workflowServiceClient.ListNamespaces(copyContext(ctx), req)
	if response != nil && response.Namespaces != nil && s.namespaceAccess != nil {
		// Even in the case of error, if there is a Namespaces list to iterate we want to remove any partial success data
		newNamespaceList := make([]*workflowservice.DescribeNamespaceResponse, 0, len(response.Namespaces))
		for _, ns := range response.Namespaces {
			if s.namespaceAccess.IsAllowed(ns.NamespaceInfo.Name) {
				newNamespaceList = append(newNamespaceList, ns)
			}
		}
		response.Namespaces = newNamespaceList
	}
	return response, err
}

// Passthrough APIs below this point

func (s *workflowServiceProxyServer) CountWorkflowExecutions(ctx context.Context, in0 *workflowservice.CountWorkflowExecutionsRequest) (*workflowservice.CountWorkflowExecutionsResponse, error) {
	return s.workflowServiceClient.CountWorkflowExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) CreateSchedule(ctx context.Context, in0 *workflowservice.CreateScheduleRequest) (*workflowservice.CreateScheduleResponse, error) {
	return s.workflowServiceClient.CreateSchedule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DeleteSchedule(ctx context.Context, in0 *workflowservice.DeleteScheduleRequest) (*workflowservice.DeleteScheduleResponse, error) {
	return s.workflowServiceClient.DeleteSchedule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DeleteWorkflowExecution(ctx context.Context, in0 *workflowservice.DeleteWorkflowExecutionRequest) (*workflowservice.DeleteWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.DeleteWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DeprecateNamespace(ctx context.Context, in0 *workflowservice.DeprecateNamespaceRequest) (*workflowservice.DeprecateNamespaceResponse, error) {
	return s.workflowServiceClient.DeprecateNamespace(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeBatchOperation(ctx context.Context, in0 *workflowservice.DescribeBatchOperationRequest) (*workflowservice.DescribeBatchOperationResponse, error) {
	return s.workflowServiceClient.DescribeBatchOperation(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeNamespace(ctx context.Context, in0 *workflowservice.DescribeNamespaceRequest) (*workflowservice.DescribeNamespaceResponse, error) {
	return s.workflowServiceClient.DescribeNamespace(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeSchedule(ctx context.Context, in0 *workflowservice.DescribeScheduleRequest) (*workflowservice.DescribeScheduleResponse, error) {
	return s.workflowServiceClient.DescribeSchedule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeTaskQueue(ctx context.Context, in0 *workflowservice.DescribeTaskQueueRequest) (*workflowservice.DescribeTaskQueueResponse, error) {
	return s.workflowServiceClient.DescribeTaskQueue(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeWorkflowExecution(ctx context.Context, in0 *workflowservice.DescribeWorkflowExecutionRequest) (*workflowservice.DescribeWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.DescribeWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ExecuteMultiOperation(ctx context.Context, in0 *workflowservice.ExecuteMultiOperationRequest) (*workflowservice.ExecuteMultiOperationResponse, error) {
	return s.workflowServiceClient.ExecuteMultiOperation(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetClusterInfo(ctx context.Context, in0 *workflowservice.GetClusterInfoRequest) (*workflowservice.GetClusterInfoResponse, error) {
	return s.workflowServiceClient.GetClusterInfo(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetSearchAttributes(ctx context.Context, in0 *workflowservice.GetSearchAttributesRequest) (*workflowservice.GetSearchAttributesResponse, error) {
	return s.workflowServiceClient.GetSearchAttributes(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetSystemInfo(ctx context.Context, in0 *workflowservice.GetSystemInfoRequest) (*workflowservice.GetSystemInfoResponse, error) {
	return s.workflowServiceClient.GetSystemInfo(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetWorkerBuildIdCompatibility(ctx context.Context, in0 *workflowservice.GetWorkerBuildIdCompatibilityRequest) (*workflowservice.GetWorkerBuildIdCompatibilityResponse, error) {
	return s.workflowServiceClient.GetWorkerBuildIdCompatibility(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetWorkerTaskReachability(ctx context.Context, in0 *workflowservice.GetWorkerTaskReachabilityRequest) (*workflowservice.GetWorkerTaskReachabilityResponse, error) {
	return s.workflowServiceClient.GetWorkerTaskReachability(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetWorkerVersioningRules(ctx context.Context, in0 *workflowservice.GetWorkerVersioningRulesRequest) (*workflowservice.GetWorkerVersioningRulesResponse, error) {
	return s.workflowServiceClient.GetWorkerVersioningRules(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetWorkflowExecutionHistory(ctx context.Context, in0 *workflowservice.GetWorkflowExecutionHistoryRequest) (*workflowservice.GetWorkflowExecutionHistoryResponse, error) {
	return s.workflowServiceClient.GetWorkflowExecutionHistory(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetWorkflowExecutionHistoryReverse(ctx context.Context, in0 *workflowservice.GetWorkflowExecutionHistoryReverseRequest) (*workflowservice.GetWorkflowExecutionHistoryReverseResponse, error) {
	return s.workflowServiceClient.GetWorkflowExecutionHistoryReverse(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListArchivedWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListArchivedWorkflowExecutionsRequest) (*workflowservice.ListArchivedWorkflowExecutionsResponse, error) {
	return s.workflowServiceClient.ListArchivedWorkflowExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListBatchOperations(ctx context.Context, in0 *workflowservice.ListBatchOperationsRequest) (*workflowservice.ListBatchOperationsResponse, error) {
	return s.workflowServiceClient.ListBatchOperations(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListClosedWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListClosedWorkflowExecutionsRequest) (*workflowservice.ListClosedWorkflowExecutionsResponse, error) {
	return s.workflowServiceClient.ListClosedWorkflowExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListOpenWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListOpenWorkflowExecutionsRequest) (*workflowservice.ListOpenWorkflowExecutionsResponse, error) {
	return s.workflowServiceClient.ListOpenWorkflowExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListScheduleMatchingTimes(ctx context.Context, in0 *workflowservice.ListScheduleMatchingTimesRequest) (*workflowservice.ListScheduleMatchingTimesResponse, error) {
	return s.workflowServiceClient.ListScheduleMatchingTimes(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListSchedules(ctx context.Context, in0 *workflowservice.ListSchedulesRequest) (*workflowservice.ListSchedulesResponse, error) {
	return s.workflowServiceClient.ListSchedules(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListTaskQueuePartitions(ctx context.Context, in0 *workflowservice.ListTaskQueuePartitionsRequest) (*workflowservice.ListTaskQueuePartitionsResponse, error) {
	return s.workflowServiceClient.ListTaskQueuePartitions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListWorkflowExecutions(ctx context.Context, in0 *workflowservice.ListWorkflowExecutionsRequest) (*workflowservice.ListWorkflowExecutionsResponse, error) {
	return s.workflowServiceClient.ListWorkflowExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PatchSchedule(ctx context.Context, in0 *workflowservice.PatchScheduleRequest) (*workflowservice.PatchScheduleResponse, error) {
	return s.workflowServiceClient.PatchSchedule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PollActivityTaskQueue(ctx context.Context, in0 *workflowservice.PollActivityTaskQueueRequest) (*workflowservice.PollActivityTaskQueueResponse, error) {
	return s.workflowServiceClient.PollActivityTaskQueue(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PollNexusTaskQueue(ctx context.Context, in0 *workflowservice.PollNexusTaskQueueRequest) (*workflowservice.PollNexusTaskQueueResponse, error) {
	return s.workflowServiceClient.PollNexusTaskQueue(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PollWorkflowExecutionUpdate(ctx context.Context, in0 *workflowservice.PollWorkflowExecutionUpdateRequest) (*workflowservice.PollWorkflowExecutionUpdateResponse, error) {
	return s.workflowServiceClient.PollWorkflowExecutionUpdate(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PollWorkflowTaskQueue(ctx context.Context, in0 *workflowservice.PollWorkflowTaskQueueRequest) (*workflowservice.PollWorkflowTaskQueueResponse, error) {
	return s.workflowServiceClient.PollWorkflowTaskQueue(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) QueryWorkflow(ctx context.Context, in0 *workflowservice.QueryWorkflowRequest) (*workflowservice.QueryWorkflowResponse, error) {
	return s.workflowServiceClient.QueryWorkflow(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RecordActivityTaskHeartbeat(ctx context.Context, in0 *workflowservice.RecordActivityTaskHeartbeatRequest) (*workflowservice.RecordActivityTaskHeartbeatResponse, error) {
	return s.workflowServiceClient.RecordActivityTaskHeartbeat(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RecordActivityTaskHeartbeatById(ctx context.Context, in0 *workflowservice.RecordActivityTaskHeartbeatByIdRequest) (*workflowservice.RecordActivityTaskHeartbeatByIdResponse, error) {
	return s.workflowServiceClient.RecordActivityTaskHeartbeatById(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RegisterNamespace(ctx context.Context, in0 *workflowservice.RegisterNamespaceRequest) (*workflowservice.RegisterNamespaceResponse, error) {
	return s.workflowServiceClient.RegisterNamespace(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RequestCancelWorkflowExecution(ctx context.Context, in0 *workflowservice.RequestCancelWorkflowExecutionRequest) (*workflowservice.RequestCancelWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.RequestCancelWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ResetStickyTaskQueue(ctx context.Context, in0 *workflowservice.ResetStickyTaskQueueRequest) (*workflowservice.ResetStickyTaskQueueResponse, error) {
	return s.workflowServiceClient.ResetStickyTaskQueue(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ResetWorkflowExecution(ctx context.Context, in0 *workflowservice.ResetWorkflowExecutionRequest) (*workflowservice.ResetWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.ResetWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCanceled(ctx context.Context, in0 *workflowservice.RespondActivityTaskCanceledRequest) (*workflowservice.RespondActivityTaskCanceledResponse, error) {
	return s.workflowServiceClient.RespondActivityTaskCanceled(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCanceledById(ctx context.Context, in0 *workflowservice.RespondActivityTaskCanceledByIdRequest) (*workflowservice.RespondActivityTaskCanceledByIdResponse, error) {
	return s.workflowServiceClient.RespondActivityTaskCanceledById(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCompleted(ctx context.Context, in0 *workflowservice.RespondActivityTaskCompletedRequest) (*workflowservice.RespondActivityTaskCompletedResponse, error) {
	return s.workflowServiceClient.RespondActivityTaskCompleted(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskCompletedById(ctx context.Context, in0 *workflowservice.RespondActivityTaskCompletedByIdRequest) (*workflowservice.RespondActivityTaskCompletedByIdResponse, error) {
	return s.workflowServiceClient.RespondActivityTaskCompletedById(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskFailed(ctx context.Context, in0 *workflowservice.RespondActivityTaskFailedRequest) (*workflowservice.RespondActivityTaskFailedResponse, error) {
	return s.workflowServiceClient.RespondActivityTaskFailed(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondActivityTaskFailedById(ctx context.Context, in0 *workflowservice.RespondActivityTaskFailedByIdRequest) (*workflowservice.RespondActivityTaskFailedByIdResponse, error) {
	return s.workflowServiceClient.RespondActivityTaskFailedById(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondNexusTaskCompleted(ctx context.Context, in0 *workflowservice.RespondNexusTaskCompletedRequest) (*workflowservice.RespondNexusTaskCompletedResponse, error) {
	return s.workflowServiceClient.RespondNexusTaskCompleted(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondNexusTaskFailed(ctx context.Context, in0 *workflowservice.RespondNexusTaskFailedRequest) (*workflowservice.RespondNexusTaskFailedResponse, error) {
	return s.workflowServiceClient.RespondNexusTaskFailed(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondQueryTaskCompleted(ctx context.Context, in0 *workflowservice.RespondQueryTaskCompletedRequest) (*workflowservice.RespondQueryTaskCompletedResponse, error) {
	return s.workflowServiceClient.RespondQueryTaskCompleted(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondWorkflowTaskCompleted(ctx context.Context, in0 *workflowservice.RespondWorkflowTaskCompletedRequest) (*workflowservice.RespondWorkflowTaskCompletedResponse, error) {
	return s.workflowServiceClient.RespondWorkflowTaskCompleted(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RespondWorkflowTaskFailed(ctx context.Context, in0 *workflowservice.RespondWorkflowTaskFailedRequest) (*workflowservice.RespondWorkflowTaskFailedResponse, error) {
	return s.workflowServiceClient.RespondWorkflowTaskFailed(copyContext(ctx), in0)
}

// nolint:staticcheck // SA1019: workflowservice.ScanWorkflowExecutionsRequest is deprecated: Use with `ListWorkflowExecutions`
func (s *workflowServiceProxyServer) ScanWorkflowExecutions(ctx context.Context, in0 *workflowservice.ScanWorkflowExecutionsRequest) (*workflowservice.ScanWorkflowExecutionsResponse, error) {
	return s.workflowServiceClient.ScanWorkflowExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) SignalWithStartWorkflowExecution(ctx context.Context, in0 *workflowservice.SignalWithStartWorkflowExecutionRequest) (*workflowservice.SignalWithStartWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.SignalWithStartWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) SignalWorkflowExecution(ctx context.Context, in0 *workflowservice.SignalWorkflowExecutionRequest) (*workflowservice.SignalWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.SignalWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) StartBatchOperation(ctx context.Context, in0 *workflowservice.StartBatchOperationRequest) (*workflowservice.StartBatchOperationResponse, error) {
	return s.workflowServiceClient.StartBatchOperation(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) StartWorkflowExecution(ctx context.Context, in0 *workflowservice.StartWorkflowExecutionRequest) (*workflowservice.StartWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.StartWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) StopBatchOperation(ctx context.Context, in0 *workflowservice.StopBatchOperationRequest) (*workflowservice.StopBatchOperationResponse, error) {
	return s.workflowServiceClient.StopBatchOperation(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) TerminateWorkflowExecution(ctx context.Context, in0 *workflowservice.TerminateWorkflowExecutionRequest) (*workflowservice.TerminateWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.TerminateWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateNamespace(ctx context.Context, in0 *workflowservice.UpdateNamespaceRequest) (*workflowservice.UpdateNamespaceResponse, error) {
	return s.workflowServiceClient.UpdateNamespace(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateSchedule(ctx context.Context, in0 *workflowservice.UpdateScheduleRequest) (*workflowservice.UpdateScheduleResponse, error) {
	return s.workflowServiceClient.UpdateSchedule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerBuildIdCompatibility(ctx context.Context, in0 *workflowservice.UpdateWorkerBuildIdCompatibilityRequest) (*workflowservice.UpdateWorkerBuildIdCompatibilityResponse, error) {
	return s.workflowServiceClient.UpdateWorkerBuildIdCompatibility(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerVersioningRules(ctx context.Context, in0 *workflowservice.UpdateWorkerVersioningRulesRequest) (*workflowservice.UpdateWorkerVersioningRulesResponse, error) {
	return s.workflowServiceClient.UpdateWorkerVersioningRules(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateWorkflowExecution(ctx context.Context, in0 *workflowservice.UpdateWorkflowExecutionRequest) (*workflowservice.UpdateWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.UpdateWorkflowExecution(copyContext(ctx), in0)
}

func copyContext(src context.Context) context.Context {
	val := metadata.ValueFromIncomingContext(src, DCRedirectionContextHeaderName)
	if len(val) > 0 {
		src = metadata.AppendToOutgoingContext(src, DCRedirectionContextHeaderName, val[0])
	}
	return src
}
