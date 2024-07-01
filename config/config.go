package config

import (
	"github.com/urfave/cli/v2"
	"google.golang.org/grpc"
)

const (
	ListenPortFlag         = "listen"
	RemoteServerRPCAddress = "remote"
)

type (
	Config interface {
		GetGRPCServerOptions() []grpc.ServerOption
		GetListenPort() int
		// RPCAddress indicate the remote service address(Host:Port). Host can be DNS name.
		GetRemoteServerRPCAddress() string
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

func (c *cliConfigProvider) GetGRPCServerOptions() []grpc.ServerOption {
	return nil
}

func (c *cliConfigProvider) GetListenPort() int {
	return c.ctx.Int(ListenPortFlag)
}

func (c *cliConfigProvider) GetRemoteServerRPCAddress() string {
	return c.ctx.String(RemoteServerRPCAddress)
}
