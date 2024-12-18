package frontend

import (
	"github.com/temporalio/s2s-proxy/client"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
)

type (
	lazyClient struct {
		clientProvider client.ClientProvider
		logger         log.Logger
	}
)

func NewLazyClient(
	clientProvider client.ClientProvider,
	logger log.Logger,
) workflowservice.WorkflowServiceClient {
	return &lazyClient{
		clientProvider: clientProvider,
		logger:         logger,
	}
}
