package config

import (
	"net"

	"github.com/temporalio/s2s-proxy/encryption"
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

	TLSLocalClientCertPathFlag = "tls-local-client-cert-path"
	TLSLocalClientKeyPathFlag  = "tls-local-client-key-path"
	TLSLocalServerCAPathFlag   = "tls-local-server-ca-path"
	TLSLocalServerNameFlag     = "tls-local-server-name"
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

		GetLocalClientTLSConfig() encryption.ClientTLSConfig

		GetRemoteClientTLSConfig() encryption.ClientTLSConfig
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

func (c *cliConfigProvider) GetLocalClientTLSConfig() encryption.ClientTLSConfig {
	return encryption.ClientTLSConfig{
		CertificatePath: c.ctx.String(TLSLocalClientCertPathFlag),
		KeyPath:         c.ctx.String(TLSLocalClientKeyPathFlag),
		ServerCAPath:    c.ctx.String(TLSLocalServerCAPathFlag),
		ServerName:      c.ctx.String(TLSLocalServerNameFlag),
	}
}

// TODO
func (c *cliConfigProvider) GetRemoteClientTLSConfig() encryption.ClientTLSConfig {
	return encryption.ClientTLSConfig{}
}
