package proxy

import (
	"go.uber.org/fx"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
)

var Module = fx.Provide(
	NewProxy,
)

func GRPCServerProvider(config config.Config) *grpc.Server {
	return grpc.NewServer(config.GetGRPCServerOptions()...)
}
