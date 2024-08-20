package proxy

import (
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
)

const (
	echoServerAddress          = "localhost:7266"
	serverProxyInboundAddress  = "localhost:7366"
	serverProxyOutboundAddress = "localhost:7466"
	echoClientAddress          = "localhost:8266"
	clientProxyInboundAddress  = "localhost:8366"
	clientProxyOutboundAddress = "localhost:8466"
	invalidAddress             = ""
)

var (
	serverClusterShard = history.ClusterShardID{
		ClusterID: 1,
		ShardID:   2,
	}
	clientClusterShard = history.ClusterShardID{
		ClusterID: 2,
		ShardID:   4,
	}
)

type (
	proxyTestSuite struct {
		suite.Suite
		ctrl *gomock.Controller
	}
)

func TestProxyTestSuite(t *testing.T) {
	suite.Run(t, new(proxyTestSuite))
}

func (s *proxyTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
}

func (s *proxyTestSuite) AfterTest(suiteName, testName string) {
}

func verifyEcho(sequence []int64, echoed map[int64]bool) bool {
	if len(sequence) != len(echoed) {
		return false
	}

	for _, n := range sequence {
		if exists := echoed[n]; !exists {
			return false
		}
	}

	return true
}

func genSequence(initial int64, n int) []int64 {
	var sequence []int64
	for i := 0; i < n; i++ {
		sequence = append(sequence, initial)
		initial++
	}

	return sequence
}

func (s *proxyTestSuite) Test_Echo_Success() {
	tests := []struct {
		echoServerInfo clusterInfo
		echoClientInfo clusterInfo
	}{
		{
			// 0: No proxy
			// echo_server <- - -> echo_client
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
			},
		},
		{
			// 1: server only proxy
			// echo_server <-> proxy.inbound <- - -> echo_client
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				proxyConfig: &proxyConfig{
					inboundServerAddress:  serverProxyInboundAddress,
					localServerAddress:    echoServerAddress,
					outboundServerAddress: serverProxyOutboundAddress,
					remoteServerAddress:   echoClientAddress,
				},
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
			},
		},
		{
			// 2: client only proxy
			// echo_server <- - -> proxy.outbound <-> echo_client
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				proxyConfig: &proxyConfig{
					inboundServerAddress:  clientProxyInboundAddress,
					localServerAddress:    echoClientAddress,
					outboundServerAddress: clientProxyOutboundAddress,
					remoteServerAddress:   echoServerAddress,
				},
			},
		},
		{
			// 3. server & client proxy
			// echo_server <-> proxy.inbound <- - -> proxy.outbound <-> echo_client
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				proxyConfig: &proxyConfig{
					inboundServerAddress:  serverProxyInboundAddress,
					localServerAddress:    echoServerAddress,
					outboundServerAddress: serverProxyOutboundAddress,
					remoteServerAddress:   clientProxyInboundAddress,
				},
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				proxyConfig: &proxyConfig{
					inboundServerAddress:  clientProxyInboundAddress,
					localServerAddress:    echoClientAddress,
					outboundServerAddress: clientProxyOutboundAddress,
					remoteServerAddress:   serverProxyInboundAddress,
				},
			},
		},
	}

	sequence := genSequence(1, 100)
	logger := log.NewTestLogger()
	for _, ts := range tests {
		echoServer := newEchoServer(ts.echoServerInfo, logger)
		echoClient := newEchoClient(ts.echoServerInfo, ts.echoClientInfo, logger)
		echoServer.start()
		echoClient.start()

		func() {
			defer func() {
				echoClient.stop()
				echoServer.stop()
			}()

			echoed, err := echoClient.sendAndRecv(sequence)
			s.NoError(err)
			s.True(verifyEcho(sequence, echoed))
		}()
	}
}
