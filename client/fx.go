package client

import (
	"net"

	"github.com/temporalio/s2s-proxy/client/rpc"

	"go.uber.org/fx"
)

var Module = fx.Provide(
	rpc.NewRPCFactory,
	GrpcListenerProvider,
	NewClientFactory,
)

func GrpcListenerProvider(factory rpc.RPCFactory) net.Listener {
	return factory.GetGRPCListener()
}
