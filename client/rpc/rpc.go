package rpc

import (
	"crypto/tls"

	"github.com/temporalio/s2s-proxy/config"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	// RPCFactory creates gRPC listener and connection.
	RPCFactory interface {
		CreateRemoteFrontendGRPCConnection(rpcAddress string, tlsConfig *tls.Config) *grpc.ClientConn
	}

	// rpcFactory is an implementation of common.rpcFactory interface
	rpcFactory struct {
		config config.Config
		logger log.Logger
	}
)

// NewFactory builds a new RPCFactory
// conforming to the underlying configuration
func NewRPCFactory(
	config config.Config,
	logger log.Logger,
) RPCFactory {
	return &rpcFactory{
		config: config,
		logger: logger,
	}
}

// CreateRemoteFrontendGRPCConnection creates connection for gRPC calls
func (d *rpcFactory) CreateRemoteFrontendGRPCConnection(rpcAddress string, tlsConfig *tls.Config) *grpc.ClientConn {
	connection, err := dial(rpcAddress, tlsConfig, d.logger)
	if err != nil {
		d.logger.Fatal("Failed to create gRPC connection", tag.Error(err))
		return nil
	}

	return connection
}
