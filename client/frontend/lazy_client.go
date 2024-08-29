package frontend

import (
	"github.com/temporalio/s2s-proxy/client"
	"go.temporal.io/api/workflowservice/v1"
)

type (
	lazyClient struct {
		clientProvider client.ClientProvider
	}
)

func NewLazyClient(
	clientProvider client.ClientProvider,
) workflowservice.WorkflowServiceClient {
	return &lazyClient{
		clientProvider: clientProvider,
	}
}
