package proxy

import (
	"go.temporal.io/server/common/log"
)

func (s *proxyTestSuite) Test_Reconnect() {
	echoServerInfo := clusterInfo{
		serverAddress:  echoServerAddress,
		clusterShardID: serverClusterShard,
		s2sProxyConfig: createEchoServerConfig(),
	}

	echoClientInfo := clusterInfo{
		serverAddress:  echoClientAddress,
		clusterShardID: clientClusterShard,
		s2sProxyConfig: createEchoClientConfig(),
	}

	logger := log.NewTestLogger()
	echoServer := newEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := newEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)
	echoServer.start()
	echoClient.start()

	echoClient.stop()
	echoServer.stop()
}
