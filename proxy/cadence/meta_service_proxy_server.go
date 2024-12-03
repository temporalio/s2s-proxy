package cadence

import (
	"context"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
)

var _ apiv1.MetaAPIYARPCServer = metaServiceProxyServer{}

func NewMetaServiceProxyServer(
	logger log.Logger,
	workflowServiceClient workflowservice.WorkflowServiceClient,
) apiv1.MetaAPIYARPCServer {
	return metaServiceProxyServer{
		workflowServiceClient: workflowServiceClient,
		logger:                logger,
	}
}

type metaServiceProxyServer struct {
	logger                log.Logger
	workflowServiceClient workflowservice.WorkflowServiceClient
}

func (m metaServiceProxyServer) Health(ctx context.Context, request *apiv1.HealthRequest) (*apiv1.HealthResponse, error) {
	//TODO implement me
	panic("implement me")
}
