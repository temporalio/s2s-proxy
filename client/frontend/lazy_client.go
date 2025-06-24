package frontend

import (
	"go.temporal.io/api/workflowservice/v1"

	"github.com/temporalio/s2s-proxy/client"
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
