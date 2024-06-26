package client

import (
	"net"

	"github.com/temporalio/temporal-proxy/client/rpc"

	"go.uber.org/fx"
)

var Module = fx.Provide(
	rpc.NewRPCFactory,
	GrpcListenerProvider,
)

func GrpcListenerProvider(factory rpc.RPCFactory) net.Listener {
	return factory.GetGRPCListener()
}
