package client

import (
	"crypto/tls"
	"fmt"
	"sync"

	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
)

type (
	// ClientFactory can be used to create RPC clients for temporal services
	ClientFactory interface {
		NewRemoteAdminClient(clientConfig config.ClientConfig) (adminservice.AdminServiceClient, error)
		NewRemoteWorkflowServiceClient(clientConfig config.ClientConfig) (workflowservice.WorkflowServiceClient, error)
	}

	clientFactory struct {
		rpcFactory rpc.RPCFactory
		logger     log.Logger
	}

	ClientProvider interface {
		GetAdminClient() (adminservice.AdminServiceClient, error)
	}

	clientProvider struct {
		clientConfig  config.ClientConfig
		clientFactory ClientFactory
		logger        log.Logger

		adminClientsLock sync.Mutex
		adminClient      adminservice.AdminServiceClient
	}
)

func NewClientProvider(
	clientConfig config.ClientConfig,
	clientFactory ClientFactory,
	logger log.Logger,
) ClientProvider {
	return &clientProvider{
		clientConfig:  clientConfig,
		clientFactory: clientFactory,
		logger:        logger,
	}
}

func (c *clientProvider) GetAdminClient() (adminservice.AdminServiceClient, error) {
	if c.adminClient == nil {
		// Create admin client
		c.adminClientsLock.Lock()
		defer c.adminClientsLock.Unlock()

		c.logger.Info(fmt.Sprintf("Create adminclient with config: %v", c.clientConfig))
		adminClient, err := c.clientFactory.NewRemoteAdminClient(c.clientConfig)
		if err != nil {
			return nil, err
		}

		c.adminClient = adminClient
	}

	return c.adminClient, nil
}

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
) (adminservice.AdminServiceClient, error) {
	var tlsConfig *tls.Config
	var err error

	if clientConfig.TLS.IsEnabled() {
		tlsConfig, err = encryption.GetClientTLSConfig(clientConfig.TLS)
		if err != nil {
			return nil, err
		}
	}

	connection, err := cf.rpcFactory.CreateRemoteFrontendGRPCConnection(clientConfig.ForwardAddress, tlsConfig)
	if err != nil {
		return nil, err
	}

	return adminservice.NewAdminServiceClient(connection), nil
}

func (cf *clientFactory) NewRemoteWorkflowServiceClient(
	clientConfig config.ClientConfig,
) (workflowservice.WorkflowServiceClient, error) {
	var tlsConfig *tls.Config
	var err error

	// TODO: refactor a bit. probably only need to load tls config once.
	if clientConfig.TLS.IsEnabled() {
		tlsConfig, err = encryption.GetClientTLSConfig(clientConfig.TLS)
		if err != nil {
			return nil, err
		}
	}

	// TODO: maybe only need to create one connection?
	connection, err := cf.rpcFactory.CreateRemoteFrontendGRPCConnection(clientConfig.ForwardAddress, tlsConfig)
	if err != nil {
		return nil, err
	}
	return workflowservice.NewWorkflowServiceClient(connection), nil
}
