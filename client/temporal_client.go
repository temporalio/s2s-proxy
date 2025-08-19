package client

import (
	"fmt"
	"sync"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport"
)

type (
	// ClientFactory can be used to create RPC clients for temporal services
	ClientFactory interface {
		NewRemoteAdminClient(clientConfig config.ProxyClientConfig) (adminservice.AdminServiceClient, error)
		NewRemoteWorkflowServiceClient(clientConfig config.ProxyClientConfig) (workflowservice.WorkflowServiceClient, error)
	}

	clientFactory struct {
		clientMetrics   *grpcprom.ClientMetrics
		clientTransport transport.ClientTransport
		logger          log.Logger
	}

	ClientProvider interface {
		GetAdminClient() (adminservice.AdminServiceClient, error)
		GetWorkflowServiceClient() (workflowservice.WorkflowServiceClient, error)
	}

	clientProvider struct {
		clientConfig  config.ProxyClientConfig
		clientFactory ClientFactory
		logger        log.Logger

		adminClientsLock sync.Mutex
		adminClient      adminservice.AdminServiceClient

		workflowserviceClientsLock sync.Mutex
		workflowserviceClient      workflowservice.WorkflowServiceClient
	}
)

func NewClientProvider(
	clientConfig config.ProxyClientConfig,
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
		c.adminClientsLock.Lock()
		defer c.adminClientsLock.Unlock()

		if c.adminClient == nil {
			c.logger.Info(fmt.Sprintf("Create adminclient with config: %v", c.clientConfig))
			adminClient, err := c.clientFactory.NewRemoteAdminClient(c.clientConfig)
			if err != nil {
				return nil, err
			}

			c.adminClient = adminClient
		}
	}

	return c.adminClient, nil
}

func (c *clientProvider) GetWorkflowServiceClient() (workflowservice.WorkflowServiceClient, error) {
	if c.workflowserviceClient == nil {
		c.workflowserviceClientsLock.Lock()
		defer c.workflowserviceClientsLock.Unlock()

		if c.workflowserviceClient == nil {
			c.logger.Info(fmt.Sprintf("Create workflowservice client with config: %v", c.clientConfig))
			workflowserviceClient, err := c.clientFactory.NewRemoteWorkflowServiceClient(c.clientConfig)
			if err != nil {
				return nil, err
			}

			c.workflowserviceClient = workflowserviceClient
		}
	}

	return c.workflowserviceClient, nil
}

// NewFactory creates an instance of client factory that knows how to dispatch RPC calls.
func NewClientFactory(
	clientTransport transport.ClientTransport,
	clientMetrics *grpcprom.ClientMetrics,
	logger log.Logger,
) ClientFactory {
	return &clientFactory{
		clientMetrics:   clientMetrics,
		clientTransport: clientTransport,
		logger:          logger,
	}
}

func (cf *clientFactory) NewRemoteAdminClient(
	clientConfig config.ProxyClientConfig, // TODO: not used. remove it.
) (adminservice.AdminServiceClient, error) {
	connection, err := cf.clientTransport.Connect(cf.clientMetrics)
	if err != nil {
		return nil, err
	}

	return adminservice.NewAdminServiceClient(connection), nil
}

func (cf *clientFactory) NewRemoteWorkflowServiceClient(
	clientConfig config.ProxyClientConfig,
) (workflowservice.WorkflowServiceClient, error) {
	connection, err := cf.clientTransport.Connect(cf.clientMetrics)
	if err != nil {
		return nil, err
	}
	return workflowservice.NewWorkflowServiceClient(connection), nil
}
