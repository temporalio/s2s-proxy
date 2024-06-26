package proxy

import (
	"go.uber.org/fx"
	"google.golang.org/grpc"

	"github.com/temporalio/temporal-proxy/config"
)

var Module = fx.Provide(
	GRPCServerProvider,
	NewProxy,
)

func GRPCServerProvider(config config.Config) *grpc.Server {
	return grpc.NewServer(config.GetGRPCServerOptions()...)
}
