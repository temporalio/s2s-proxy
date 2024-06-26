package config

import (
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
)

const (
	GRPCPortFlag = "port"
)

type (
	Config interface {
		GetGRPCPort() int
		GetGRPCServerOptions() []grpc.ServerOption
	}

	cliConfigProvider struct {
		ctx *cli.Context
	}
)

func newConfigProvider(ctx *cli.Context) Config {
	return &cliConfigProvider{
		ctx: ctx,
	}
}

func (c *cliConfigProvider) GetGRPCPort() int {
	return c.ctx.Int(GRPCPortFlag)
}

func (c *cliConfigProvider) GetGRPCServerOptions() []grpc.ServerOption {
	return nil
}
