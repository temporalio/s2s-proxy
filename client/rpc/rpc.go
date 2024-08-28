package rpc

import (
	"crypto/tls"

	"github.com/temporalio/s2s-proxy/config"

	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
)

type (
	// RPCFactory creates gRPC listener and connection.
	RPCFactory interface {
		CreateRemoteFrontendGRPCConnection(rpcAddress string, tlsConfig *tls.Config) (*grpc.ClientConn, error)
	}

	// rpcFactory is an implementation of common.rpcFactory interface
	rpcFactory struct {
		configProvider config.ConfigProvider
		logger         log.Logger
	}
)

// NewFactory builds a new RPCFactory
// conforming to the underlying configuration
func NewRPCFactory(
	configProvider config.ConfigProvider,
	logger log.Logger,
) RPCFactory {
	return &rpcFactory{
		configProvider: configProvider,
		logger:         logger,
	}
}

// CreateRemoteFrontendGRPCConnection creates connection for gRPC calls
func (d *rpcFactory) CreateRemoteFrontendGRPCConnection(rpcAddress string, tlsConfig *tls.Config) (*grpc.ClientConn, error) {
	connection, err := dial(rpcAddress, tlsConfig, d.logger)
	if err != nil {
		return nil, err
	}

	return connection, nil
}
