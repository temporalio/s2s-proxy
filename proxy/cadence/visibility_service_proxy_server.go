package cadence

import (
	"context"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
)

var _ apiv1.VisibilityAPIYARPCServer = visibilityServiceProxyServer{}

func NewVisibilityServiceProxyServer(
	logger log.Logger,
	workflowServiceClient workflowservice.WorkflowServiceClient,
) apiv1.VisibilityAPIYARPCServer {
	return visibilityServiceProxyServer{
		logger:                logger,
		workflowServiceClient: workflowServiceClient,
	}
}

type visibilityServiceProxyServer struct {
	logger                log.Logger
	workflowServiceClient workflowservice.WorkflowServiceClient
}

func (v visibilityServiceProxyServer) ListWorkflowExecutions(ctx context.Context, request *apiv1.ListWorkflowExecutionsRequest) (*apiv1.ListWorkflowExecutionsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (v visibilityServiceProxyServer) ListOpenWorkflowExecutions(ctx context.Context, request *apiv1.ListOpenWorkflowExecutionsRequest) (*apiv1.ListOpenWorkflowExecutionsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (v visibilityServiceProxyServer) ListClosedWorkflowExecutions(ctx context.Context, request *apiv1.ListClosedWorkflowExecutionsRequest) (*apiv1.ListClosedWorkflowExecutionsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (v visibilityServiceProxyServer) ListArchivedWorkflowExecutions(ctx context.Context, request *apiv1.ListArchivedWorkflowExecutionsRequest) (*apiv1.ListArchivedWorkflowExecutionsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (v visibilityServiceProxyServer) ScanWorkflowExecutions(ctx context.Context, request *apiv1.ScanWorkflowExecutionsRequest) (*apiv1.ScanWorkflowExecutionsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (v visibilityServiceProxyServer) CountWorkflowExecutions(ctx context.Context, request *apiv1.CountWorkflowExecutionsRequest) (*apiv1.CountWorkflowExecutionsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (v visibilityServiceProxyServer) GetSearchAttributes(ctx context.Context, request *apiv1.GetSearchAttributesRequest) (*apiv1.GetSearchAttributesResponse, error) {
	//TODO implement me
	panic("implement me")
}
