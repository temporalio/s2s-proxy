package operator

import (
	"github.com/temporalio/s2s-proxy/client"
	"go.temporal.io/api/operatorservice/v1"
)

type (
	lazyClient struct {
		clientProvider client.ClientProvider
	}
)

func NewLazyClient(
	clientProvider client.ClientProvider,
) operatorservice.OperatorServiceClient {
	return &lazyClient{
		clientProvider: clientProvider,
	}
}
