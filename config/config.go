package config

import (
	"net"

	"github.com/urfave/cli/v2"
	"go.temporal.io/server/common/convert"
	"google.golang.org/grpc"
)

const (
	OutboundPortFlag           = "outbound-port"
	InboundPortFlag            = "inbound-port"
	RemoteServerRPCAddressFlag = "remote"
	LocalServerRPCAddressFlag  = "local"
	// Localhost default hostname
	LocalhostIPDefault = "127.0.0.1"
)

type (
	Config interface {
		GetGRPCServerOptions() []grpc.ServerOption

		// RPCAddress indicate the server address(Host:Port) serving outbound traffic from local server.
		GetOutboundServerAddress() string

		// RPCAddress indicate the server address(Host:Port) serving inbound traffic from remote server.
		GetInboundServerAddress() string

		// RPCAddress indicate the remote service address(Host:Port). Host can be DNS name.
		GetRemoteServerRPCAddress() string

		// RPCAddress indicate the local service address(Host:Port). Host can be DNS name.
		GetLocalServerRPCAddress() string
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

func (c *cliConfigProvider) GetOutboundServerAddress() string {
	port := convert.IntToString(c.ctx.Int(OutboundPortFlag))
	return net.JoinHostPort(LocalhostIPDefault, port)
}

func (c *cliConfigProvider) GetInboundServerAddress() string {
	port := convert.IntToString(c.ctx.Int(InboundPortFlag))
	return net.JoinHostPort(LocalhostIPDefault, port)
}

func (c *cliConfigProvider) GetRemoteServerRPCAddress() string {
	return c.ctx.String(RemoteServerRPCAddressFlag)
}

func (c *cliConfigProvider) GetLocalServerRPCAddress() string {
	return c.ctx.String(LocalServerRPCAddressFlag)
}
