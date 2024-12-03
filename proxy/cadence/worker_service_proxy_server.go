package cadence

import (
	"context"
	"github.com/temporalio/s2s-proxy/client"
	feclient "github.com/temporalio/s2s-proxy/client/frontend"
	"github.com/temporalio/s2s-proxy/config"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
)

type workerServiceProxyServer struct {
	workflowServiceClient workflowservice.WorkflowServiceClient
	logger                log.Logger
}

var _ apiv1.WorkerAPIYARPCServer = workerServiceProxyServer{}

func NewWorkerServiceProxyServer(
	clientConfig config.ProxyClientConfig,
	clientFactory client.ClientFactory,
	logger log.Logger,
) apiv1.WorkerAPIYARPCServer {
	clientProvider := client.NewClientProvider(clientConfig, clientFactory, logger)

	return workerServiceProxyServer{
		workflowServiceClient: feclient.NewLazyClient(clientProvider),
		logger:                logger,
	}
}

func (w workerServiceProxyServer) PollForDecisionTask(ctx context.Context, req *apiv1.PollForDecisionTaskRequest) (*apiv1.PollForDecisionTaskResponse, error) {
	w.logger.Info("Cadence API server: PollForDecisionTask called.")
	tReq := &workflowservice.PollWorkflowTaskQueueRequest{
		Namespace: req.GetDomain(),
		TaskQueue: &taskqueue.TaskQueue{
			Name: req.GetTaskList().GetName(),
			Kind: toTemporalTaskQueueKind(req.GetTaskList().GetKind()),
		},
		Identity: req.GetIdentity(),
	}

	resp, err := w.workflowServiceClient.PollWorkflowTaskQueue(ctx, tReq)
	return toPollForDecisionTaskResponse(resp, err)
}

func toPollForDecisionTaskResponse(resp *workflowservice.PollWorkflowTaskQueueResponse, err error) (*apiv1.PollForDecisionTaskResponse, error) {
	if err != nil {
		return nil, err
	}

	return &apiv1.PollForDecisionTaskResponse{
		TaskToken:              resp.GetTaskToken(),
		WorkflowExecution:      toCadenceWorkflowExecution(resp.GetWorkflowExecution()),
		WorkflowType:           toCadenceWorkflowType(resp.GetWorkflowType()),
		PreviousStartedEventId: toInt64ValuePtr(resp.GetPreviousStartedEventId()),
		StartedEventId:         resp.GetStartedEventId(),
	}, nil
}

func (w workerServiceProxyServer) RespondDecisionTaskCompleted(ctx context.Context, request *apiv1.RespondDecisionTaskCompletedRequest) (*apiv1.RespondDecisionTaskCompletedResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RespondDecisionTaskFailed(ctx context.Context, request *apiv1.RespondDecisionTaskFailedRequest) (*apiv1.RespondDecisionTaskFailedResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) PollForActivityTask(ctx context.Context, request *apiv1.PollForActivityTaskRequest) (*apiv1.PollForActivityTaskResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskCompleted(ctx context.Context, request *apiv1.RespondActivityTaskCompletedRequest) (*apiv1.RespondActivityTaskCompletedResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskCompletedByID(ctx context.Context, request *apiv1.RespondActivityTaskCompletedByIDRequest) (*apiv1.RespondActivityTaskCompletedByIDResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskFailed(ctx context.Context, request *apiv1.RespondActivityTaskFailedRequest) (*apiv1.RespondActivityTaskFailedResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskFailedByID(ctx context.Context, request *apiv1.RespondActivityTaskFailedByIDRequest) (*apiv1.RespondActivityTaskFailedByIDResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskCanceled(ctx context.Context, request *apiv1.RespondActivityTaskCanceledRequest) (*apiv1.RespondActivityTaskCanceledResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskCanceledByID(ctx context.Context, request *apiv1.RespondActivityTaskCanceledByIDRequest) (*apiv1.RespondActivityTaskCanceledByIDResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RecordActivityTaskHeartbeat(ctx context.Context, request *apiv1.RecordActivityTaskHeartbeatRequest) (*apiv1.RecordActivityTaskHeartbeatResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RecordActivityTaskHeartbeatByID(ctx context.Context, request *apiv1.RecordActivityTaskHeartbeatByIDRequest) (*apiv1.RecordActivityTaskHeartbeatByIDResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) RespondQueryTaskCompleted(ctx context.Context, request *apiv1.RespondQueryTaskCompletedRequest) (*apiv1.RespondQueryTaskCompletedResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (w workerServiceProxyServer) ResetStickyTaskList(ctx context.Context, request *apiv1.ResetStickyTaskListRequest) (*apiv1.ResetStickyTaskListResponse, error) {
	//TODO implement me
	panic("implement me")
}
