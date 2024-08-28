package client

import (
	"crypto/tls"

	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
)

type (
	// ClientFactory can be used to create RPC clients for temporal services
	ClientFactory interface {
		NewRemoteAdminClient(clientConfig config.ClientConfig) adminservice.AdminServiceClient
	}

	clientFactory struct {
		rpcFactory rpc.RPCFactory
		logger     log.Logger
	}
)

// NewFactory creates an instance of client factory that knows how to dispatch RPC calls.
func NewClientFactory(
	rpcFactory rpc.RPCFactory,
	logger log.Logger,
) ClientFactory {
	return &clientFactory{
		rpcFactory: rpcFactory,
		logger:     logger,
	}
}

func (cf *clientFactory) NewRemoteAdminClient(
	clientConfig config.ClientConfig,
) adminservice.AdminServiceClient {
	var tlsConfig *tls.Config
	var err error

	if clientConfig.TLS.IsEnabled() {
		tlsConfig, err = encryption.GetClientTLSConfig(clientConfig.TLS)
		if err != nil {
			cf.logger.Fatal("Failed to get client TLS config")
			return nil
		}
	}

	connection := cf.rpcFactory.CreateRemoteFrontendGRPCConnection(clientConfig.ForwardAddress, tlsConfig)
	return adminservice.NewAdminServiceClient(connection)
}
