package client

import (
	"github.com/temporalio/s2s-proxy/client/rpc"

	"go.uber.org/fx"
)

var Module = fx.Provide(
	rpc.NewRPCFactory,
	NewClientFactory,
)
