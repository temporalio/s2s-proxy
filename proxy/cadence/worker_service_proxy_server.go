package cadence

import (
	"context"
	"github.com/temporalio/s2s-proxy/proxy/cadence/cadencetype"
	"github.com/temporalio/s2s-proxy/proxy/cadence/temporaltype"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
)

type workerServiceProxyServer struct {
	workflowServiceClient workflowservice.WorkflowServiceClient
	logger                log.Logger
}

var _ apiv1.WorkerAPIYARPCServer = workerServiceProxyServer{}

func NewWorkerServiceProxyServer(
	logger log.Logger,
	workflowServiceClient workflowservice.WorkflowServiceClient,
) apiv1.WorkerAPIYARPCServer {
	return workerServiceProxyServer{
		workflowServiceClient: workflowServiceClient,
		logger:                logger,
	}
}

func (w workerServiceProxyServer) PollForDecisionTask(ctx context.Context, request *apiv1.PollForDecisionTaskRequest) (*apiv1.PollForDecisionTaskResponse, error) {
	w.logger.Info("Cadence API server: PollForDecisionTask called.")
	tReq := temporaltype.PollWorkflowTaskQueueRequest(request)
	resp, err := w.workflowServiceClient.PollWorkflowTaskQueue(ctx, tReq)
	cresp := cadencetype.PollForDecisionTaskResponse(resp)
	return cresp, cadencetype.Error(err)
}

func (w workerServiceProxyServer) RespondDecisionTaskCompleted(ctx context.Context, request *apiv1.RespondDecisionTaskCompletedRequest) (*apiv1.RespondDecisionTaskCompletedResponse, error) {
	w.logger.Info("Cadence API server: RespondDecisionTaskCompleted called.")
	tReq := temporaltype.RespondWorkflowTaskCompletedRequest(request)
	resp, err := w.workflowServiceClient.RespondWorkflowTaskCompleted(ctx, tReq)
	return cadencetype.RespondDecisionTaskCompletedResponse(resp), cadencetype.Error(err)
}

func (w workerServiceProxyServer) RespondDecisionTaskFailed(ctx context.Context, request *apiv1.RespondDecisionTaskFailedRequest) (*apiv1.RespondDecisionTaskFailedResponse, error) {
	w.logger.Info("Cadence API server: RespondDecisionTaskFailed called.")
	panic("implement me")
}

func (w workerServiceProxyServer) PollForActivityTask(ctx context.Context, request *apiv1.PollForActivityTaskRequest) (*apiv1.PollForActivityTaskResponse, error) {
	w.logger.Info("Cadence API server: PollForActivityTask called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskCompleted(ctx context.Context, request *apiv1.RespondActivityTaskCompletedRequest) (*apiv1.RespondActivityTaskCompletedResponse, error) {
	w.logger.Info("Cadence API server: RespondActivityTaskCompleted called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskCompletedByID(ctx context.Context, request *apiv1.RespondActivityTaskCompletedByIDRequest) (*apiv1.RespondActivityTaskCompletedByIDResponse, error) {
	w.logger.Info("Cadence API server: RespondActivityTaskCompletedByID called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskFailed(ctx context.Context, request *apiv1.RespondActivityTaskFailedRequest) (*apiv1.RespondActivityTaskFailedResponse, error) {
	w.logger.Info("Cadence API server: RespondActivityTaskFailed called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskFailedByID(ctx context.Context, request *apiv1.RespondActivityTaskFailedByIDRequest) (*apiv1.RespondActivityTaskFailedByIDResponse, error) {
	w.logger.Info("Cadence API server: RespondActivityTaskFailedByID called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskCanceled(ctx context.Context, request *apiv1.RespondActivityTaskCanceledRequest) (*apiv1.RespondActivityTaskCanceledResponse, error) {
	w.logger.Info("Cadence API server: RespondActivityTaskCanceled called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RespondActivityTaskCanceledByID(ctx context.Context, request *apiv1.RespondActivityTaskCanceledByIDRequest) (*apiv1.RespondActivityTaskCanceledByIDResponse, error) {
	w.logger.Info("Cadence API server: RespondActivityTaskCanceledByID called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RecordActivityTaskHeartbeat(ctx context.Context, request *apiv1.RecordActivityTaskHeartbeatRequest) (*apiv1.RecordActivityTaskHeartbeatResponse, error) {
	w.logger.Info("Cadence API server: RecordActivityTaskHeartbeat called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RecordActivityTaskHeartbeatByID(ctx context.Context, request *apiv1.RecordActivityTaskHeartbeatByIDRequest) (*apiv1.RecordActivityTaskHeartbeatByIDResponse, error) {
	w.logger.Info("Cadence API server: RecordActivityTaskHeartbeatByID called.")
	panic("implement me")
}

func (w workerServiceProxyServer) RespondQueryTaskCompleted(ctx context.Context, request *apiv1.RespondQueryTaskCompletedRequest) (*apiv1.RespondQueryTaskCompletedResponse, error) {
	w.logger.Info("Cadence API server: RespondQueryTaskCompleted called.")
	panic("implement me")
}

func (w workerServiceProxyServer) ResetStickyTaskList(ctx context.Context, request *apiv1.ResetStickyTaskListRequest) (*apiv1.ResetStickyTaskListResponse, error) {
	w.logger.Info("Cadence API server: ResetStickyTaskList called.")
	panic("implement me")
}
