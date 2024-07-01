package client

import (
	"github.com/temporalio/s2s-proxy/client/rpc"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
)

type (
	// ClientFactory can be used to create RPC clients for temporal services
	ClientFactory interface {
		NewRemoteAdminClient(rpcAddress string) adminservice.AdminServiceClient
	}

	clientFactory struct {
		rpcFactory rpc.RPCFactory
		logger     log.Logger
	}
)

// NewFactory creates an instance of client factory that knows how to dispatch RPC calls.
func NewClientFactory(
	rpcFactory rpc.RPCFactory,
	logger log.Logger,
) ClientFactory {
	return &clientFactory{
		rpcFactory: rpcFactory,
		logger:     logger,
	}
}

func (cf *clientFactory) NewRemoteAdminClient(
	rpcAddress string,
) adminservice.AdminServiceClient {
	connection := cf.rpcFactory.CreateRemoteFrontendGRPCConnection(rpcAddress)
	return adminservice.NewAdminServiceClient(connection)
}
