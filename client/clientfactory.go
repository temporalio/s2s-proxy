package client

import (
	"time"

	"github.com/temporalio/s2s-proxy/client/admin"
	"github.com/temporalio/s2s-proxy/client/rpc"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/metrics"
)

type (
	// Factory can be used to create RPC clients for temporal services
	Factory interface {
		NewRemoteAdminClientWithTimeout(rpcAddress string, timeout time.Duration, largeTimeout time.Duration) adminservice.AdminServiceClient
	}

	rpcClientFactory struct {
		rpcFactory      rpc.RPCFactory
		metricsHandler  metrics.Handler
		logger          log.Logger
		throttledLogger log.Logger
	}
)

// NewFactory creates an instance of client factory that knows how to dispatch RPC calls.
func NewClientFactory(
	rpcFactory common.RPCFactory,
	logger log.Logger,
	throttledLogger log.Logger,
) Factory {
	return &rpcClientFactory{
		rpcFactory:      rpcFactory,
		logger:          logger,
		throttledLogger: throttledLogger,
	}
}

func (cf *rpcClientFactory) NewRemoteAdminClientWithTimeout(
	rpcAddress string,
	timeout time.Duration,
	largeTimeout time.Duration,
) adminservice.AdminServiceClient {
	connection := cf.rpcFactory.CreateRemoteFrontendGRPCConnection(rpcAddress)
	client := adminservice.NewAdminServiceClient(connection)
	return cf.newAdminClient(client, timeout, largeTimeout)
}

func (cf *rpcClientFactory) newAdminClient(
	client adminservice.AdminServiceClient,
	timeout time.Duration,
	longPollTimeout time.Duration,
) adminservice.AdminServiceClient {
	client = admin.NewClient(timeout, longPollTimeout, client)
	if cf.metricsHandler != nil {
		client = admin.NewMetricClient(client, cf.metricsHandler, cf.throttledLogger)
	}
	return client
}
