package proxy

import (
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

type (
	mockConfigProvider struct {
		proxyConfig config.S2SProxyConfig
	}

	clusterInfo struct {
		serverAddress  string
		clusterShardID history.ClusterShardID
		s2sProxyConfig *config.S2SProxyConfig // if provided, used for setting up proxy
	}
	echoServer struct {
		server      *s2sproxy.TemporalAPIServer
		proxy       *s2sproxy.Proxy
		clusterInfo clusterInfo
	}
)

func (mc *mockConfigProvider) GetS2SProxyConfig() config.S2SProxyConfig {
	return mc.proxyConfig
}

// Echo server for testing stream replication. It acts as stream sender.
// It handles StreamWorkflowReplicationMessages call from Echo client (acts as stream receiver) by
// echoing back InclusiveLowWatermark in SyncReplicationState message.
func newEchoServer(
	localClusterInfo clusterInfo,
	remoteClusterInfo clusterInfo,
	logger log.Logger,
) *echoServer {
	senderAdminService := &echoAdminService{
		serviceName: "EchoServer",
		logger:      log.With(logger, common.ServiceTag("EchoServer"), tag.Address(localClusterInfo.serverAddress)),
	}
	senderWorkflowService := &echoWorkflowService{
		serviceName: "EchoServer",
		logger:      log.With(logger, common.ServiceTag("EchoServer"), tag.Address(localClusterInfo.serverAddress)),
	}

	var proxy *s2sproxy.Proxy
	var err error

	if localCfg := localClusterInfo.s2sProxyConfig; localCfg != nil {
		if remoteCfg := remoteClusterInfo.s2sProxyConfig; remoteCfg != nil {
			localCfg.Outbound.Client.ForwardAddress = remoteCfg.Inbound.Server.ListenAddress
		} else {
			localCfg.Outbound.Client.ForwardAddress = remoteClusterInfo.serverAddress
		}

		configProvider := &mockConfigProvider{
			proxyConfig: *localClusterInfo.s2sProxyConfig,
		}

		rpcFactory := rpc.NewRPCFactory(configProvider, logger)
		clientFactory := client.NewClientFactory(rpcFactory, logger)
		proxy, err = s2sproxy.NewProxy(
			configProvider,
			logger,
			clientFactory,
		)

		if err != nil {
			logger.Fatal("Failed to create proxy", tag.Error(err))
		}
	}

	return &echoServer{
		server: s2sproxy.NewTemporalAPIServer(
			"EchoServer",
			config.ServerConfig{
				ListenAddress: localClusterInfo.serverAddress,
			},
			senderAdminService,
			senderWorkflowService,
			nil,
			logger),
		proxy:       proxy,
		clusterInfo: localClusterInfo,
	}
}

func (s *echoServer) start() {
	_ = s.server.Start()
	if s.proxy != nil {
		_ = s.proxy.Start()
	}
}

func (s *echoServer) stop() {
	if s.proxy != nil {
		s.proxy.Stop()
	}
	s.server.Stop()
}