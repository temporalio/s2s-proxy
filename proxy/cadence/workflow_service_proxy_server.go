package cadence

import (
	"context"
	"github.com/gogo/protobuf/types"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
	"go.uber.org/yarpc/yarpcerrors"
)

type (
	workflowServiceProxyServer struct {
		workflowServiceClient workflowservice.WorkflowServiceClient
		logger                log.Logger
	}
)

func (s workflowServiceProxyServer) RestartWorkflowExecution(ctx context.Context, request *apiv1.RestartWorkflowExecutionRequest) (*apiv1.RestartWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) StartWorkflowExecution(ctx context.Context, request *apiv1.StartWorkflowExecutionRequest) (*apiv1.StartWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) StartWorkflowExecutionAsync(ctx context.Context, request *apiv1.StartWorkflowExecutionAsyncRequest) (*apiv1.StartWorkflowExecutionAsyncResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) SignalWorkflowExecution(ctx context.Context, request *apiv1.SignalWorkflowExecutionRequest) (*apiv1.SignalWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) SignalWithStartWorkflowExecution(ctx context.Context, request *apiv1.SignalWithStartWorkflowExecutionRequest) (*apiv1.SignalWithStartWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) SignalWithStartWorkflowExecutionAsync(ctx context.Context, request *apiv1.SignalWithStartWorkflowExecutionAsyncRequest) (*apiv1.SignalWithStartWorkflowExecutionAsyncResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) ResetWorkflowExecution(ctx context.Context, request *apiv1.ResetWorkflowExecutionRequest) (*apiv1.ResetWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) RequestCancelWorkflowExecution(ctx context.Context, request *apiv1.RequestCancelWorkflowExecutionRequest) (*apiv1.RequestCancelWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) TerminateWorkflowExecution(ctx context.Context, request *apiv1.TerminateWorkflowExecutionRequest) (*apiv1.TerminateWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) GetTaskListsByDomain(ctx context.Context, request *apiv1.GetTaskListsByDomainRequest) (*apiv1.GetTaskListsByDomainResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) GetWorkflowExecutionHistory(ctx context.Context, request *apiv1.GetWorkflowExecutionHistoryRequest) (*apiv1.GetWorkflowExecutionHistoryResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) RefreshWorkflowTasks(ctx context.Context, request *apiv1.RefreshWorkflowTasksRequest) (*apiv1.RefreshWorkflowTasksResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (s workflowServiceProxyServer) DiagnoseWorkflowExecution(ctx context.Context, request *apiv1.DiagnoseWorkflowExecutionRequest) (*apiv1.DiagnoseWorkflowExecutionResponse, error) {
	//TODO implement me
	panic("implement me")
}

func NewWorkflowServiceProxyServer(
	logger log.Logger,
	workflowServiceClient workflowservice.WorkflowServiceClient,
) apiv1.WorkflowAPIYARPCServer {
	return workflowServiceProxyServer{
		workflowServiceClient: workflowServiceClient,
		logger:                logger,
	}
}

var _ apiv1.WorkflowAPIYARPCServer = workflowServiceProxyServer{}

//func (s workflowServiceProxyServer) StartWorkflowExecution(ctx context.Context, req *apiv1.StartWorkflowExecutionRequest) (*apiv1.StartWorkflowExecutionResponse, error) {
//	return toStartWorkflowExecutionResponse(s.workflowServiceClient.StartWorkflowExecution(ctx, toStartWorkflowExecutionRequest(req)))
//}

func toStartWorkflowExecutionRequest(req *apiv1.StartWorkflowExecutionRequest) *workflowservice.StartWorkflowExecutionRequest {
	return &workflowservice.StartWorkflowExecutionRequest{
		Namespace:  req.GetDomain(),
		WorkflowId: req.GetWorkflowId(),
		WorkflowType: &common.WorkflowType{
			Name: req.GetWorkflowType().GetName(),
		},
		TaskQueue: &taskqueue.TaskQueue{
			Name: req.GetTaskList().GetName(),
			Kind: enums.TASK_QUEUE_KIND_NORMAL,
		},
	}
}

func toStartWorkflowExecutionResponse(resp *workflowservice.StartWorkflowExecutionResponse, err error) (*apiv1.StartWorkflowExecutionResponse, error) {
	return &apiv1.StartWorkflowExecutionResponse{
		RunId: resp.GetRunId(),
	}, nil
}

func toListWorkflowExecutionsRequest(req *apiv1.ListWorkflowExecutionsRequest) *workflowservice.ListWorkflowExecutionsRequest {
	return &workflowservice.ListWorkflowExecutionsRequest{
		Namespace:     req.GetDomain(),
		PageSize:      req.GetPageSize(),
		Query:         req.GetQuery(),
		NextPageToken: req.GetNextPageToken(),
	}
}

func toListWorkflowExecutionsResponse(resp *workflowservice.ListWorkflowExecutionsResponse, err error) (*apiv1.ListWorkflowExecutionsResponse, error) {
	if err != nil {
		return nil, err
	}

	return &apiv1.ListWorkflowExecutionsResponse{
		Executions:    toCadenceWorkflowExecutionInfos(resp.GetExecutions()),
		NextPageToken: resp.GetNextPageToken(),
	}, nil
}

func toCadenceWorkflowExecutionInfos(infos []*workflow.WorkflowExecutionInfo) []*apiv1.WorkflowExecutionInfo {
	result := make([]*apiv1.WorkflowExecutionInfo, len(infos))
	for i, info := range infos {
		result[i] = toCadenceWorkflowExecutionInfo(info)
	}
	return result
}

func toCadenceWorkflowExecutionInfo(info *workflow.WorkflowExecutionInfo) *apiv1.WorkflowExecutionInfo {
	return &apiv1.WorkflowExecutionInfo{
		WorkflowExecution: toCadenceExecution(info.GetExecution()),
		Type: &apiv1.WorkflowType{
			Name: info.GetType().GetName(),
		},
		StartTime:     &types.Timestamp{Seconds: info.GetStartTime().GetSeconds()},
		CloseTime:     &types.Timestamp{Seconds: info.GetCloseTime().GetSeconds()},
		CloseStatus:   toCadenceCloseStatus(info.GetStatus()),
		HistoryLength: info.GetHistoryLength(),
		TaskList:      info.GetTaskQueue(),
	}
}

func toCadenceCloseStatus(status enums.WorkflowExecutionStatus) apiv1.WorkflowExecutionCloseStatus {
	switch status {
	case enums.WORKFLOW_EXECUTION_STATUS_COMPLETED:
		return apiv1.WorkflowExecutionCloseStatus_WORKFLOW_EXECUTION_CLOSE_STATUS_COMPLETED
	case enums.WORKFLOW_EXECUTION_STATUS_FAILED:
		return apiv1.WorkflowExecutionCloseStatus_WORKFLOW_EXECUTION_CLOSE_STATUS_FAILED
	case enums.WORKFLOW_EXECUTION_STATUS_CANCELED:
		return apiv1.WorkflowExecutionCloseStatus_WORKFLOW_EXECUTION_CLOSE_STATUS_CANCELED
	case enums.WORKFLOW_EXECUTION_STATUS_TERMINATED:
		return apiv1.WorkflowExecutionCloseStatus_WORKFLOW_EXECUTION_CLOSE_STATUS_TERMINATED
	case enums.WORKFLOW_EXECUTION_STATUS_CONTINUED_AS_NEW:
		return apiv1.WorkflowExecutionCloseStatus_WORKFLOW_EXECUTION_CLOSE_STATUS_CONTINUED_AS_NEW
	case enums.WORKFLOW_EXECUTION_STATUS_TIMED_OUT:
		return apiv1.WorkflowExecutionCloseStatus_WORKFLOW_EXECUTION_CLOSE_STATUS_TIMED_OUT
	default:
		return apiv1.WorkflowExecutionCloseStatus_WORKFLOW_EXECUTION_CLOSE_STATUS_INVALID
	}
}

func toCadenceExecution(execution *common.WorkflowExecution) *apiv1.WorkflowExecution {
	return &apiv1.WorkflowExecution{
		WorkflowId: execution.GetWorkflowId(),
		RunId:      execution.GetRunId(),
	}
}

func (s workflowServiceProxyServer) ListArchivedWorkflowExecutions(ctx context.Context, req *apiv1.ListArchivedWorkflowExecutionsRequest) (*apiv1.ListArchivedWorkflowExecutionsResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method ListArchivedWorkflowExecutions not implemented")
}

func (s workflowServiceProxyServer) ScanWorkflowExecutions(ctx context.Context, req *apiv1.ScanWorkflowExecutionsRequest) (*apiv1.ScanWorkflowExecutionsResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method ScanWorkflowExecutions not implemented")
}

func (s workflowServiceProxyServer) CountWorkflowExecutions(ctx context.Context, req *apiv1.CountWorkflowExecutionsRequest) (*apiv1.CountWorkflowExecutionsResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method CountWorkflowExecutions not implemented")
}

func (s workflowServiceProxyServer) GetSearchAttributes(ctx context.Context, req *apiv1.GetSearchAttributesRequest) (*apiv1.GetSearchAttributesResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method GetSearchAttributes not implemented")
}

func (s workflowServiceProxyServer) RespondQueryTaskCompleted(ctx context.Context, req *apiv1.RespondQueryTaskCompletedRequest) (*apiv1.RespondQueryTaskCompletedResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method RespondQueryTaskCompleted not implemented")
}

func (s workflowServiceProxyServer) ResetStickyTaskList(ctx context.Context, req *apiv1.ResetStickyTaskListRequest) (*apiv1.ResetStickyTaskListResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method ResetStickyTaskList not implemented")
}

func (s workflowServiceProxyServer) QueryWorkflow(ctx context.Context, req *apiv1.QueryWorkflowRequest) (*apiv1.QueryWorkflowResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method QueryWorkflow not implemented")
}

func (s workflowServiceProxyServer) DescribeWorkflowExecution(ctx context.Context, req *apiv1.DescribeWorkflowExecutionRequest) (*apiv1.DescribeWorkflowExecutionResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method DescribeWorkflowExecution not implemented")
}

func (s workflowServiceProxyServer) DescribeTaskList(ctx context.Context, req *apiv1.DescribeTaskListRequest) (*apiv1.DescribeTaskListResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method DescribeTaskList not implemented")
}

func (s workflowServiceProxyServer) GetClusterInfo(ctx context.Context, req *apiv1.GetClusterInfoRequest) (*apiv1.GetClusterInfoResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method GetClusterInfo not implemented")
}

func (s workflowServiceProxyServer) ListTaskListPartitions(ctx context.Context, req *apiv1.ListTaskListPartitionsRequest) (*apiv1.ListTaskListPartitionsResponse, error) {
	// Implement the method logic here
	return nil, yarpcerrors.UnimplementedErrorf("method ListTaskListPartitions not implemented")
}
