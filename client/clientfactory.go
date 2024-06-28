package client

import (
	"time"

	"github.com/temporalio/s2s-proxy/client/admin"
	"github.com/temporalio/s2s-proxy/client/rpc"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
)

type (
	// ClientFactory can be used to create RPC clients for temporal services
	ClientFactory interface {
		NewRemoteAdminClientWithTimeout(rpcAddress string, timeout time.Duration, largeTimeout time.Duration) adminservice.AdminServiceClient
	}

	rpcClientFactory struct {
		rpcFactory rpc.RPCFactory
		logger     log.Logger
	}
)

// NewFactory creates an instance of client factory that knows how to dispatch RPC calls.
func NewClientFactory(
	rpcFactory rpc.RPCFactory,
	logger log.Logger,
) ClientFactory {
	return &rpcClientFactory{
		rpcFactory: rpcFactory,
		logger:     logger,
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
	return admin.NewClient(timeout, longPollTimeout, client)
}
