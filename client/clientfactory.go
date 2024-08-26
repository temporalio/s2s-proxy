package client

import (
	"crypto/tls"
	"encoding/json"
	"fmt"

	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/encryption"

	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
)

type (
	// ClientFactory can be used to create RPC clients for temporal services
	ClientFactory interface {
		NewRemoteAdminClient(rpcAddress string, tlsConfig encryption.ClientTLSConfig) adminservice.AdminServiceClient
	}

	clientFactory struct {
		rpcFactory        rpc.RPCFactory
		tlsConfigProvider encryption.TLSConfigProvider
		logger            log.Logger
	}
)

// NewFactory creates an instance of client factory that knows how to dispatch RPC calls.
func NewClientFactory(
	rpcFactory rpc.RPCFactory,
	tlsConfigProvider encryption.TLSConfigProvider,
	logger log.Logger,
) ClientFactory {
	return &clientFactory{
		rpcFactory:        rpcFactory,
		tlsConfigProvider: tlsConfigProvider,
		logger:            logger,
	}
}

func (cf *clientFactory) NewRemoteAdminClient(
	rpcAddress string,
	clientConfig encryption.ClientTLSConfig,
) adminservice.AdminServiceClient {
	var tlsConfig *tls.Config
	var err error

	if data, err := json.Marshal(clientConfig); err == nil {
		cf.logger.Info(fmt.Sprintf("AdminClientConfig: address: %s, tlsConfig(enabled=%v): %s", rpcAddress, clientConfig.IsEnabled(), string(data)))
	} else {
		cf.logger.Error(fmt.Sprintf("AdminClientConfig: address: %s, tlsConfig error: %v", rpcAddress, err))
		return nil
	}

	if clientConfig.IsEnabled() {
		tlsConfig, err = cf.tlsConfigProvider.GetClientTLSConfig(clientConfig)
		if err != nil {
			cf.logger.Fatal("Failed to get client TLS config")
			return nil
		}
	}

	connection := cf.rpcFactory.CreateRemoteFrontendGRPCConnection(rpcAddress, tlsConfig)
	return adminservice.NewAdminServiceClient(connection)
}
