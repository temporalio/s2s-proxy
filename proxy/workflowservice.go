package proxy

import (
	"context"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/logging"
)

const DCRedirectionContextHeaderName = "xdc-redirection" // https://github.com/temporalio/temporal/blob/9a1060c4162ff62576cb899d7e5b1bae179af814/common/rpc/interceptor/redirection.go#L27

// PreservedHeaders are forwarded from the incoming context to the outgoing
// context so they survive the proxy hop.
var PreservedHeaders = []string{
	DCRedirectionContextHeaderName,
	common.RequestTranslationHeaderName,
}

type responseConstructorFn func() any

var (
	// https://github.com/temporalio/temporal/blob/f93f567d8395042b132ff69685053e0282e15416/common/rpc/interceptor/redirection.go#L47

	globalAPIResponses = map[string]responseConstructorFn{
		"DescribeTaskQueue":                  func() any { return &workflowservice.DescribeTaskQueueResponse{} },
		"DescribeWorkflowExecution":          func() any { return &workflowservice.DescribeWorkflowExecutionResponse{} },
		"GetWorkflowExecutionHistory":        func() any { return &workflowservice.GetWorkflowExecutionHistoryResponse{} },
		"GetWorkflowExecutionHistoryReverse": func() any { return &workflowservice.GetWorkflowExecutionHistoryReverseResponse{} },
		"ListArchivedWorkflowExecutions":     func() any { return &workflowservice.ListArchivedWorkflowExecutionsResponse{} },
		"ListClosedWorkflowExecutions":       func() any { return &workflowservice.ListClosedWorkflowExecutionsResponse{} },
		"ListOpenWorkflowExecutions":         func() any { return &workflowservice.ListOpenWorkflowExecutionsResponse{} },
		"ListWorkflowExecutions":             func() any { return &workflowservice.ListWorkflowExecutionsResponse{} },
		"ScanWorkflowExecutions":             func() any { return &workflowservice.ScanWorkflowExecutionsResponse{} }, //nolint:staticcheck // deprecated upstream but still forwarded for old clients
		"CountWorkflowExecutions":            func() any { return &workflowservice.CountWorkflowExecutionsResponse{} },
		"PollActivityTaskQueue":              func() any { return &workflowservice.PollActivityTaskQueueResponse{} },
		"PollWorkflowTaskQueue":              func() any { return &workflowservice.PollWorkflowTaskQueueResponse{} },
		"PollNexusTaskQueue":                 func() any { return &workflowservice.PollNexusTaskQueueResponse{} },
		"QueryWorkflow":                      func() any { return &workflowservice.QueryWorkflowResponse{} },
		"RecordActivityTaskHeartbeat":        func() any { return &workflowservice.RecordActivityTaskHeartbeatResponse{} },
		"RecordActivityTaskHeartbeatById":    func() any { return &workflowservice.RecordActivityTaskHeartbeatByIdResponse{} },
		"RequestCancelWorkflowExecution":     func() any { return &workflowservice.RequestCancelWorkflowExecutionResponse{} },
		"ResetStickyTaskQueue":               func() any { return &workflowservice.ResetStickyTaskQueueResponse{} },
		"ShutdownWorker":                     func() any { return &workflowservice.ShutdownWorkerResponse{} },
		"ResetWorkflowExecution":             func() any { return &workflowservice.ResetWorkflowExecutionResponse{} },
		"RespondActivityTaskCanceled":        func() any { return &workflowservice.RespondActivityTaskCanceledResponse{} },
		"RespondActivityTaskCanceledById":    func() any { return &workflowservice.RespondActivityTaskCanceledByIdResponse{} },
		"RespondActivityTaskCompleted":       func() any { return &workflowservice.RespondActivityTaskCompletedResponse{} },
		"RespondActivityTaskCompletedById":   func() any { return &workflowservice.RespondActivityTaskCompletedByIdResponse{} },
		"RespondActivityTaskFailed":          func() any { return &workflowservice.RespondActivityTaskFailedResponse{} },
		"RespondActivityTaskFailedById":      func() any { return &workflowservice.RespondActivityTaskFailedByIdResponse{} },
		"RespondWorkflowTaskCompleted":       func() any { return &workflowservice.RespondWorkflowTaskCompletedResponse{} },
		"RespondWorkflowTaskFailed":          func() any { return &workflowservice.RespondWorkflowTaskFailedResponse{} },
		"RespondQueryTaskCompleted":          func() any { return &workflowservice.RespondQueryTaskCompletedResponse{} },
		"RespondNexusTaskCompleted":          func() any { return &workflowservice.RespondNexusTaskCompletedResponse{} },
		"RespondNexusTaskFailed":             func() any { return &workflowservice.RespondNexusTaskFailedResponse{} },
		"SignalWithStartWorkflowExecution":   func() any { return &workflowservice.SignalWithStartWorkflowExecutionResponse{} },
		"SignalWorkflowExecution":            func() any { return &workflowservice.SignalWorkflowExecutionResponse{} },
		"StartWorkflowExecution":             func() any { return &workflowservice.StartWorkflowExecutionResponse{} },
		"ExecuteMultiOperation":              func() any { return &workflowservice.ExecuteMultiOperationResponse{} },
		"UpdateWorkflowExecution":            func() any { return &workflowservice.UpdateWorkflowExecutionResponse{} },
		"PollWorkflowExecutionUpdate":        func() any { return &workflowservice.PollWorkflowExecutionUpdateResponse{} },
		"TerminateWorkflowExecution":         func() any { return &workflowservice.TerminateWorkflowExecutionResponse{} },
		"DeleteWorkflowExecution":            func() any { return &workflowservice.DeleteWorkflowExecutionResponse{} },
		"ListTaskQueuePartitions":            func() any { return &workflowservice.ListTaskQueuePartitionsResponse{} },
		"PauseWorkflowExecution":             func() any { return &workflowservice.PauseWorkflowExecutionResponse{} },
		"UnpauseWorkflowExecution":           func() any { return &workflowservice.UnpauseWorkflowExecutionResponse{} },

		"CreateSchedule":                   func() any { return &workflowservice.CreateScheduleResponse{} },
		"DescribeSchedule":                 func() any { return &workflowservice.DescribeScheduleResponse{} },
		"UpdateSchedule":                   func() any { return &workflowservice.UpdateScheduleResponse{} },
		"PatchSchedule":                    func() any { return &workflowservice.PatchScheduleResponse{} },
		"DeleteSchedule":                   func() any { return &workflowservice.DeleteScheduleResponse{} },
		"ListSchedules":                    func() any { return &workflowservice.ListSchedulesResponse{} },
		"CountSchedules":                   func() any { return &workflowservice.CountSchedulesResponse{} },
		"ListScheduleMatchingTimes":        func() any { return &workflowservice.ListScheduleMatchingTimesResponse{} },
		"UpdateWorkerBuildIdCompatibility": func() any { return &workflowservice.UpdateWorkerBuildIdCompatibilityResponse{} },
		"GetWorkerBuildIdCompatibility":    func() any { return &workflowservice.GetWorkerBuildIdCompatibilityResponse{} },
		"UpdateWorkerVersioningRules":      func() any { return &workflowservice.UpdateWorkerVersioningRulesResponse{} },
		"GetWorkerVersioningRules":         func() any { return &workflowservice.GetWorkerVersioningRulesResponse{} },
		"GetWorkerTaskReachability":        func() any { return &workflowservice.GetWorkerTaskReachabilityResponse{} },

		"StartBatchOperation":            func() any { return &workflowservice.StartBatchOperationResponse{} },
		"StopBatchOperation":             func() any { return &workflowservice.StopBatchOperationResponse{} },
		"DescribeBatchOperation":         func() any { return &workflowservice.DescribeBatchOperationResponse{} },
		"ListBatchOperations":            func() any { return &workflowservice.ListBatchOperationsResponse{} },
		"UpdateActivityOptions":          func() any { return &workflowservice.UpdateActivityOptionsResponse{} },
		"PauseActivity":                  func() any { return &workflowservice.PauseActivityResponse{} },
		"UnpauseActivity":                func() any { return &workflowservice.UnpauseActivityResponse{} },
		"ResetActivity":                  func() any { return &workflowservice.ResetActivityResponse{} },
		"UpdateActivityExecutionOptions": func() any { return &workflowservice.UpdateActivityExecutionOptionsResponse{} },
		"PauseActivityExecution":         func() any { return &workflowservice.PauseActivityExecutionResponse{} },
		"UnpauseActivityExecution":       func() any { return &workflowservice.UnpauseActivityExecutionResponse{} },
		"ResetActivityExecution":         func() any { return &workflowservice.ResetActivityExecutionResponse{} },
		"UpdateWorkflowExecutionOptions": func() any { return &workflowservice.UpdateWorkflowExecutionOptionsResponse{} },

		"DescribeDeployment":                           func() any { return &workflowservice.DescribeDeploymentResponse{} },        // [cleanup-wv-pre-release]
		"ListDeployments":                              func() any { return &workflowservice.ListDeploymentsResponse{} },           // [cleanup-wv-pre-release]
		"GetDeploymentReachability":                    func() any { return &workflowservice.GetDeploymentReachabilityResponse{} }, // [cleanup-wv-pre-release]
		"GetCurrentDeployment":                         func() any { return &workflowservice.GetCurrentDeploymentResponse{} },      // [cleanup-wv-pre-release]
		"SetCurrentDeployment":                         func() any { return &workflowservice.SetCurrentDeploymentResponse{} },      // [cleanup-wv-pre-release]
		"DescribeWorkerDeployment":                     func() any { return &workflowservice.DescribeWorkerDeploymentResponse{} },
		"DescribeWorkerDeploymentVersion":              func() any { return &workflowservice.DescribeWorkerDeploymentVersionResponse{} },
		"SetWorkerDeploymentCurrentVersion":            func() any { return &workflowservice.SetWorkerDeploymentCurrentVersionResponse{} },
		"SetWorkerDeploymentRampingVersion":            func() any { return &workflowservice.SetWorkerDeploymentRampingVersionResponse{} },
		"SetWorkerDeploymentManager":                   func() any { return &workflowservice.SetWorkerDeploymentManagerResponse{} },
		"ListWorkerDeployments":                        func() any { return &workflowservice.ListWorkerDeploymentsResponse{} },
		"CreateWorkerDeployment":                       func() any { return &workflowservice.CreateWorkerDeploymentResponse{} },
		"DeleteWorkerDeployment":                       func() any { return &workflowservice.DeleteWorkerDeploymentResponse{} },
		"CreateWorkerDeploymentVersion":                func() any { return &workflowservice.CreateWorkerDeploymentVersionResponse{} },
		"UpdateWorkerDeploymentVersionComputeConfig":   func() any { return &workflowservice.UpdateWorkerDeploymentVersionComputeConfigResponse{} },
		"ValidateWorkerDeploymentVersionComputeConfig": func() any { return &workflowservice.ValidateWorkerDeploymentVersionComputeConfigResponse{} },
		"DeleteWorkerDeploymentVersion":                func() any { return &workflowservice.DeleteWorkerDeploymentVersionResponse{} },
		"UpdateWorkerDeploymentVersionMetadata":        func() any { return &workflowservice.UpdateWorkerDeploymentVersionMetadataResponse{} },

		"CreateWorkflowRule":    func() any { return &workflowservice.CreateWorkflowRuleResponse{} },
		"DescribeWorkflowRule":  func() any { return &workflowservice.DescribeWorkflowRuleResponse{} },
		"DeleteWorkflowRule":    func() any { return &workflowservice.DeleteWorkflowRuleResponse{} },
		"ListWorkflowRules":     func() any { return &workflowservice.ListWorkflowRulesResponse{} },
		"TriggerWorkflowRule":   func() any { return &workflowservice.TriggerWorkflowRuleResponse{} },
		"RecordWorkerHeartbeat": func() any { return &workflowservice.RecordWorkerHeartbeatResponse{} },
		"ListWorkers":           func() any { return &workflowservice.ListWorkersResponse{} },
		"CountWorkers":          func() any { return &workflowservice.CountWorkersResponse{} },
		"DescribeWorker":        func() any { return &workflowservice.DescribeWorkerResponse{} },
		"UpdateTaskQueueConfig": func() any { return &workflowservice.UpdateTaskQueueConfigResponse{} },
		"FetchWorkerConfig":     func() any { return &workflowservice.FetchWorkerConfigResponse{} },
		"UpdateWorkerConfig":    func() any { return &workflowservice.UpdateWorkerConfigResponse{} },

		"StartActivityExecution":         func() any { return &workflowservice.StartActivityExecutionResponse{} },
		"CountActivityExecutions":        func() any { return &workflowservice.CountActivityExecutionsResponse{} },
		"ListActivityExecutions":         func() any { return &workflowservice.ListActivityExecutionsResponse{} },
		"DescribeActivityExecution":      func() any { return &workflowservice.DescribeActivityExecutionResponse{} },
		"PollActivityExecution":          func() any { return &workflowservice.PollActivityExecutionResponse{} },
		"RequestCancelActivityExecution": func() any { return &workflowservice.RequestCancelActivityExecutionResponse{} },
		"TerminateActivityExecution":     func() any { return &workflowservice.TerminateActivityExecutionResponse{} },
		"DeleteActivityExecution":        func() any { return &workflowservice.DeleteActivityExecutionResponse{} },

		"CountNexusOperationExecutions":        func() any { return &workflowservice.CountNexusOperationExecutionsResponse{} },
		"DeleteNexusOperationExecution":        func() any { return &workflowservice.DeleteNexusOperationExecutionResponse{} },
		"DescribeNexusOperationExecution":      func() any { return &workflowservice.DescribeNexusOperationExecutionResponse{} },
		"ListNexusOperationExecutions":         func() any { return &workflowservice.ListNexusOperationExecutionsResponse{} },
		"PollNexusOperationExecution":          func() any { return &workflowservice.PollNexusOperationExecutionResponse{} },
		"RequestCancelNexusOperationExecution": func() any { return &workflowservice.RequestCancelNexusOperationExecutionResponse{} },
		"StartNexusOperationExecution":         func() any { return &workflowservice.StartNexusOperationExecutionResponse{} },
		"TerminateNexusOperationExecution":     func() any { return &workflowservice.TerminateNexusOperationExecutionResponse{} },
	}
)

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
	loggers logging.LoggerProvider,
) workflowservice.WorkflowServiceServer {
	logger := log.With(loggers.Get(logging.WorkflowService), common.ServiceTag(serviceName))
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

func (s *workflowServiceProxyServer) ShutdownWorker(ctx context.Context, in0 *workflowservice.ShutdownWorkerRequest) (*workflowservice.ShutdownWorkerResponse, error) {
	return s.workflowServiceClient.ShutdownWorker(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) CountSchedules(ctx context.Context, in0 *workflowservice.CountSchedulesRequest) (*workflowservice.CountSchedulesResponse, error) {
	return s.workflowServiceClient.CountSchedules(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeDeployment(ctx context.Context, in0 *workflowservice.DescribeDeploymentRequest) (*workflowservice.DescribeDeploymentResponse, error) {
	return s.workflowServiceClient.DescribeDeployment(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeWorkerDeploymentVersion(ctx context.Context, in0 *workflowservice.DescribeWorkerDeploymentVersionRequest) (*workflowservice.DescribeWorkerDeploymentVersionResponse, error) {
	return s.workflowServiceClient.DescribeWorkerDeploymentVersion(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListDeployments(ctx context.Context, in0 *workflowservice.ListDeploymentsRequest) (*workflowservice.ListDeploymentsResponse, error) {
	return s.workflowServiceClient.ListDeployments(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetDeploymentReachability(ctx context.Context, in0 *workflowservice.GetDeploymentReachabilityRequest) (*workflowservice.GetDeploymentReachabilityResponse, error) {
	return s.workflowServiceClient.GetDeploymentReachability(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) GetCurrentDeployment(ctx context.Context, in0 *workflowservice.GetCurrentDeploymentRequest) (*workflowservice.GetCurrentDeploymentResponse, error) {
	return s.workflowServiceClient.GetCurrentDeployment(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) SetCurrentDeployment(ctx context.Context, in0 *workflowservice.SetCurrentDeploymentRequest) (*workflowservice.SetCurrentDeploymentResponse, error) {
	return s.workflowServiceClient.SetCurrentDeployment(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) SetWorkerDeploymentCurrentVersion(ctx context.Context, in0 *workflowservice.SetWorkerDeploymentCurrentVersionRequest) (*workflowservice.SetWorkerDeploymentCurrentVersionResponse, error) {
	return s.workflowServiceClient.SetWorkerDeploymentCurrentVersion(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeWorkerDeployment(ctx context.Context, in0 *workflowservice.DescribeWorkerDeploymentRequest) (*workflowservice.DescribeWorkerDeploymentResponse, error) {
	return s.workflowServiceClient.DescribeWorkerDeployment(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DeleteWorkerDeployment(ctx context.Context, in0 *workflowservice.DeleteWorkerDeploymentRequest) (*workflowservice.DeleteWorkerDeploymentResponse, error) {
	return s.workflowServiceClient.DeleteWorkerDeployment(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DeleteWorkerDeploymentVersion(ctx context.Context, in0 *workflowservice.DeleteWorkerDeploymentVersionRequest) (*workflowservice.DeleteWorkerDeploymentVersionResponse, error) {
	return s.workflowServiceClient.DeleteWorkerDeploymentVersion(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) SetWorkerDeploymentRampingVersion(ctx context.Context, in0 *workflowservice.SetWorkerDeploymentRampingVersionRequest) (*workflowservice.SetWorkerDeploymentRampingVersionResponse, error) {
	return s.workflowServiceClient.SetWorkerDeploymentRampingVersion(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListWorkerDeployments(ctx context.Context, in0 *workflowservice.ListWorkerDeploymentsRequest) (*workflowservice.ListWorkerDeploymentsResponse, error) {
	return s.workflowServiceClient.ListWorkerDeployments(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) CreateWorkerDeployment(ctx context.Context, in0 *workflowservice.CreateWorkerDeploymentRequest) (*workflowservice.CreateWorkerDeploymentResponse, error) {
	return s.workflowServiceClient.CreateWorkerDeployment(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) CreateWorkerDeploymentVersion(ctx context.Context, in0 *workflowservice.CreateWorkerDeploymentVersionRequest) (*workflowservice.CreateWorkerDeploymentVersionResponse, error) {
	return s.workflowServiceClient.CreateWorkerDeploymentVersion(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerDeploymentVersionComputeConfig(ctx context.Context, in0 *workflowservice.UpdateWorkerDeploymentVersionComputeConfigRequest) (*workflowservice.UpdateWorkerDeploymentVersionComputeConfigResponse, error) {
	return s.workflowServiceClient.UpdateWorkerDeploymentVersionComputeConfig(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ValidateWorkerDeploymentVersionComputeConfig(ctx context.Context, in0 *workflowservice.ValidateWorkerDeploymentVersionComputeConfigRequest) (*workflowservice.ValidateWorkerDeploymentVersionComputeConfigResponse, error) {
	return s.workflowServiceClient.ValidateWorkerDeploymentVersionComputeConfig(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerDeploymentVersionMetadata(ctx context.Context, in0 *workflowservice.UpdateWorkerDeploymentVersionMetadataRequest) (*workflowservice.UpdateWorkerDeploymentVersionMetadataResponse, error) {
	return s.workflowServiceClient.UpdateWorkerDeploymentVersionMetadata(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) SetWorkerDeploymentManager(ctx context.Context, in0 *workflowservice.SetWorkerDeploymentManagerRequest) (*workflowservice.SetWorkerDeploymentManagerResponse, error) {
	return s.workflowServiceClient.SetWorkerDeploymentManager(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateActivityOptions(ctx context.Context, in0 *workflowservice.UpdateActivityOptionsRequest) (*workflowservice.UpdateActivityOptionsResponse, error) {
	return s.workflowServiceClient.UpdateActivityOptions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateWorkflowExecutionOptions(ctx context.Context, in0 *workflowservice.UpdateWorkflowExecutionOptionsRequest) (*workflowservice.UpdateWorkflowExecutionOptionsResponse, error) {
	return s.workflowServiceClient.UpdateWorkflowExecutionOptions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PauseActivity(ctx context.Context, in0 *workflowservice.PauseActivityRequest) (*workflowservice.PauseActivityResponse, error) {
	return s.workflowServiceClient.PauseActivity(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UnpauseActivity(ctx context.Context, in0 *workflowservice.UnpauseActivityRequest) (*workflowservice.UnpauseActivityResponse, error) {
	return s.workflowServiceClient.UnpauseActivity(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ResetActivity(ctx context.Context, in0 *workflowservice.ResetActivityRequest) (*workflowservice.ResetActivityResponse, error) {
	return s.workflowServiceClient.ResetActivity(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) CreateWorkflowRule(ctx context.Context, in0 *workflowservice.CreateWorkflowRuleRequest) (*workflowservice.CreateWorkflowRuleResponse, error) {
	return s.workflowServiceClient.CreateWorkflowRule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeWorkflowRule(ctx context.Context, in0 *workflowservice.DescribeWorkflowRuleRequest) (*workflowservice.DescribeWorkflowRuleResponse, error) {
	return s.workflowServiceClient.DescribeWorkflowRule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DeleteWorkflowRule(ctx context.Context, in0 *workflowservice.DeleteWorkflowRuleRequest) (*workflowservice.DeleteWorkflowRuleResponse, error) {
	return s.workflowServiceClient.DeleteWorkflowRule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListWorkflowRules(ctx context.Context, in0 *workflowservice.ListWorkflowRulesRequest) (*workflowservice.ListWorkflowRulesResponse, error) {
	return s.workflowServiceClient.ListWorkflowRules(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) TriggerWorkflowRule(ctx context.Context, in0 *workflowservice.TriggerWorkflowRuleRequest) (*workflowservice.TriggerWorkflowRuleResponse, error) {
	return s.workflowServiceClient.TriggerWorkflowRule(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RecordWorkerHeartbeat(ctx context.Context, in0 *workflowservice.RecordWorkerHeartbeatRequest) (*workflowservice.RecordWorkerHeartbeatResponse, error) {
	return s.workflowServiceClient.RecordWorkerHeartbeat(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListWorkers(ctx context.Context, in0 *workflowservice.ListWorkersRequest) (*workflowservice.ListWorkersResponse, error) {
	return s.workflowServiceClient.ListWorkers(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) CountWorkers(ctx context.Context, in0 *workflowservice.CountWorkersRequest) (*workflowservice.CountWorkersResponse, error) {
	return s.workflowServiceClient.CountWorkers(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateTaskQueueConfig(ctx context.Context, in0 *workflowservice.UpdateTaskQueueConfigRequest) (*workflowservice.UpdateTaskQueueConfigResponse, error) {
	return s.workflowServiceClient.UpdateTaskQueueConfig(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) FetchWorkerConfig(ctx context.Context, in0 *workflowservice.FetchWorkerConfigRequest) (*workflowservice.FetchWorkerConfigResponse, error) {
	return s.workflowServiceClient.FetchWorkerConfig(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateWorkerConfig(ctx context.Context, in0 *workflowservice.UpdateWorkerConfigRequest) (*workflowservice.UpdateWorkerConfigResponse, error) {
	return s.workflowServiceClient.UpdateWorkerConfig(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeWorker(ctx context.Context, in0 *workflowservice.DescribeWorkerRequest) (*workflowservice.DescribeWorkerResponse, error) {
	return s.workflowServiceClient.DescribeWorker(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PauseWorkflowExecution(ctx context.Context, in0 *workflowservice.PauseWorkflowExecutionRequest) (*workflowservice.PauseWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.PauseWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UnpauseWorkflowExecution(ctx context.Context, in0 *workflowservice.UnpauseWorkflowExecutionRequest) (*workflowservice.UnpauseWorkflowExecutionResponse, error) {
	return s.workflowServiceClient.UnpauseWorkflowExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) StartActivityExecution(ctx context.Context, in0 *workflowservice.StartActivityExecutionRequest) (*workflowservice.StartActivityExecutionResponse, error) {
	return s.workflowServiceClient.StartActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) StartNexusOperationExecution(ctx context.Context, in0 *workflowservice.StartNexusOperationExecutionRequest) (*workflowservice.StartNexusOperationExecutionResponse, error) {
	return s.workflowServiceClient.StartNexusOperationExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeActivityExecution(ctx context.Context, in0 *workflowservice.DescribeActivityExecutionRequest) (*workflowservice.DescribeActivityExecutionResponse, error) {
	return s.workflowServiceClient.DescribeActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DescribeNexusOperationExecution(ctx context.Context, in0 *workflowservice.DescribeNexusOperationExecutionRequest) (*workflowservice.DescribeNexusOperationExecutionResponse, error) {
	return s.workflowServiceClient.DescribeNexusOperationExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PollActivityExecution(ctx context.Context, in0 *workflowservice.PollActivityExecutionRequest) (*workflowservice.PollActivityExecutionResponse, error) {
	return s.workflowServiceClient.PollActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PollNexusOperationExecution(ctx context.Context, in0 *workflowservice.PollNexusOperationExecutionRequest) (*workflowservice.PollNexusOperationExecutionResponse, error) {
	return s.workflowServiceClient.PollNexusOperationExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListActivityExecutions(ctx context.Context, in0 *workflowservice.ListActivityExecutionsRequest) (*workflowservice.ListActivityExecutionsResponse, error) {
	return s.workflowServiceClient.ListActivityExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ListNexusOperationExecutions(ctx context.Context, in0 *workflowservice.ListNexusOperationExecutionsRequest) (*workflowservice.ListNexusOperationExecutionsResponse, error) {
	return s.workflowServiceClient.ListNexusOperationExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) CountActivityExecutions(ctx context.Context, in0 *workflowservice.CountActivityExecutionsRequest) (*workflowservice.CountActivityExecutionsResponse, error) {
	return s.workflowServiceClient.CountActivityExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) CountNexusOperationExecutions(ctx context.Context, in0 *workflowservice.CountNexusOperationExecutionsRequest) (*workflowservice.CountNexusOperationExecutionsResponse, error) {
	return s.workflowServiceClient.CountNexusOperationExecutions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RequestCancelActivityExecution(ctx context.Context, in0 *workflowservice.RequestCancelActivityExecutionRequest) (*workflowservice.RequestCancelActivityExecutionResponse, error) {
	return s.workflowServiceClient.RequestCancelActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) RequestCancelNexusOperationExecution(ctx context.Context, in0 *workflowservice.RequestCancelNexusOperationExecutionRequest) (*workflowservice.RequestCancelNexusOperationExecutionResponse, error) {
	return s.workflowServiceClient.RequestCancelNexusOperationExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) TerminateActivityExecution(ctx context.Context, in0 *workflowservice.TerminateActivityExecutionRequest) (*workflowservice.TerminateActivityExecutionResponse, error) {
	return s.workflowServiceClient.TerminateActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DeleteActivityExecution(ctx context.Context, in0 *workflowservice.DeleteActivityExecutionRequest) (*workflowservice.DeleteActivityExecutionResponse, error) {
	return s.workflowServiceClient.DeleteActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) PauseActivityExecution(ctx context.Context, in0 *workflowservice.PauseActivityExecutionRequest) (*workflowservice.PauseActivityExecutionResponse, error) {
	return s.workflowServiceClient.PauseActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) ResetActivityExecution(ctx context.Context, in0 *workflowservice.ResetActivityExecutionRequest) (*workflowservice.ResetActivityExecutionResponse, error) {
	return s.workflowServiceClient.ResetActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UnpauseActivityExecution(ctx context.Context, in0 *workflowservice.UnpauseActivityExecutionRequest) (*workflowservice.UnpauseActivityExecutionResponse, error) {
	return s.workflowServiceClient.UnpauseActivityExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) UpdateActivityExecutionOptions(ctx context.Context, in0 *workflowservice.UpdateActivityExecutionOptionsRequest) (*workflowservice.UpdateActivityExecutionOptionsResponse, error) {
	return s.workflowServiceClient.UpdateActivityExecutionOptions(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) TerminateNexusOperationExecution(ctx context.Context, in0 *workflowservice.TerminateNexusOperationExecutionRequest) (*workflowservice.TerminateNexusOperationExecutionResponse, error) {
	return s.workflowServiceClient.TerminateNexusOperationExecution(copyContext(ctx), in0)
}

func (s *workflowServiceProxyServer) DeleteNexusOperationExecution(ctx context.Context, in0 *workflowservice.DeleteNexusOperationExecutionRequest) (*workflowservice.DeleteNexusOperationExecutionResponse, error) {
	return s.workflowServiceClient.DeleteNexusOperationExecution(copyContext(ctx), in0)
}

func copyContext(src context.Context) context.Context {
	for _, header := range PreservedHeaders {
		val := metadata.ValueFromIncomingContext(src, header)
		if len(val) > 0 {
			src = metadata.AppendToOutgoingContext(src, header, val[0])
		}
	}
	return src
}
