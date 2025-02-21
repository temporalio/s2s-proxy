package proxy

import (
	"context"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

const (
	streamServerAddress = "localhost:5555"
)

func (s *proxyTestSuite) Test_StreamServer() {
	serviceName := "streamServer"
	localClusterInfo := clusterInfo{
		serverAddress: streamServerAddress,
	}

	ns := map[string]bool{}
	logger := log.NewTestLogger()
	adminService := &streamTestAdminService{
		serviceName: serviceName,
		logger:      log.With(logger, common.ServiceTag(serviceName), tag.Address(localClusterInfo.serverAddress)),
		namespaces:  ns,
		payloadSize: defaultPayloadSize,
	}

	workflowService := &echoWorkflowService{
		serviceName: serviceName,
		logger:      log.With(logger, common.ServiceTag(serviceName), tag.Address(localClusterInfo.serverAddress)),
	}

	serverConfig := config.ProxyServerConfig{
		TCPServerSetting: config.TCPServerSetting{
			ListenAddress: streamServerAddress,
		},
	}

	tm := transport.NewTransportManager(&config.EmptyConfigProvider, logger)
	serverTransport, err := tm.OpenServer(serverConfig)
	if err != nil {
		logger.Fatal("Failed to create server transport", tag.Error(err))
	}

	clientConfig := config.ProxyClientConfig{
		TCPClientSetting: config.TCPClientSetting{
			ServerAddress: localClusterInfo.serverAddress,
		},
	}

	clientTransport, err := tm.OpenClient(clientConfig)
	if err != nil {
		logger.Fatal("Failed to create client transport", tag.Error(err))
	}

	clientLogger := log.With(logger, common.ServiceTag("streamClient"))
	factory := client.NewClientFactory(clientTransport, clientLogger)
	clientProvider := client.NewClientProvider(clientConfig, factory, clientLogger)

	server := s2sproxy.NewTemporalAPIServer(
		serviceName,
		serverConfig,
		adminService,
		workflowService,
		nil,
		serverTransport,
		logger)

	server.Start()
	defer server.Stop()

	adminClient, err := clientProvider.GetAdminClient()
	s.NoError(err)

	ctx := context.TODO()
	_, err = retry(func() (adminservice.AdminService_StreamWorkflowReplicationMessagesClient, error) {
		return adminClient.StreamWorkflowReplicationMessages(ctx)
	}, 5, clientLogger)

	s.NoError(err)
	clientLogger.Info("client goroutine completed")
}
