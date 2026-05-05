package proxy

import (
	"testing"

	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/endtoendtest"
)

func createEchoServerConfigWithPorts(
	echoServerAddress string,
	serverProxyInboundAddress string,
	serverProxyOutboundAddress string,
	opts ...cfgOption,
) *config.S2SProxyConfig {
	return createS2SProxyConfig(&config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{{
			Name: "proxy1",
			Local: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpServer:      config.TCPTLSInfo{ConnectionString: serverProxyOutboundAddress},
				TcpClient:      config.TCPTLSInfo{ConnectionString: echoServerAddress},
			},
			Remote: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpServer:      config.TCPTLSInfo{ConnectionString: serverProxyInboundAddress},
				TcpClient:      config.TCPTLSInfo{ConnectionString: "to-be-added"},
			},
		}},
	}, opts)
}

func createEchoClientConfigWithPorts(
	echoClientAddress string,
	clientProxyInboundAddress string,
	clientProxyOutboundAddress string,
	opts ...cfgOption,
) *config.S2SProxyConfig {
	return createS2SProxyConfig(&config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{{
			Name: "proxy2",
			Local: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpServer:      config.TCPTLSInfo{ConnectionString: clientProxyOutboundAddress},
				TcpClient:      config.TCPTLSInfo{ConnectionString: echoClientAddress},
			},
			Remote: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpServer:      config.TCPTLSInfo{ConnectionString: clientProxyInboundAddress},
				TcpClient:      config.TCPTLSInfo{ConnectionString: "to-be-added"},
			},
		}},
	}, opts)
}

func benchmarkStreamSendRecvWithoutProxy(b *testing.B, payloadSize int) {
	echoServerInfo := endtoendtest.ClusterInfo{
		ServerAddress:  GetLocalhostAddress(),
		ClusterShardID: serverClusterShard,
	}

	echoClientInfo := endtoendtest.ClusterInfo{
		ServerAddress:  GetLocalhostAddress(),
		ClusterShardID: clientClusterShard,
	}

	runSendRecvBench(b, echoServerInfo, echoClientInfo, payloadSize)
}

func benchmarkStreamSendRecvWithMuxProxy(b *testing.B, payloadSize int) {
	b.Log("Start BenchmarkStreamSendRecv")

	// Allocate ports dynamically
	echoServerAddress := GetLocalhostAddress()
	serverProxyInboundAddress := GetLocalhostAddress()
	serverProxyOutboundAddress := GetLocalhostAddress()
	echoClientAddress := GetLocalhostAddress()
	clientProxyInboundAddress := GetLocalhostAddress()
	clientProxyOutboundAddress := GetLocalhostAddress()

	echoServerConfig := createEchoServerConfigWithPorts(
		echoServerAddress,
		serverProxyInboundAddress,
		serverProxyOutboundAddress,
		withRemoteMuxClient(clientProxyInboundAddress),
	)

	echoClientConfig := createEchoClientConfigWithPorts(
		echoClientAddress,
		clientProxyInboundAddress,
		clientProxyOutboundAddress,
		withRemoteMuxServer(clientProxyInboundAddress),
	)

	echoServerInfo := endtoendtest.ClusterInfo{
		ServerAddress:  echoServerAddress,
		ClusterShardID: serverClusterShard,
		S2sProxyConfig: echoServerConfig,
	}
	echoClientInfo := endtoendtest.ClusterInfo{
		ServerAddress:  echoClientAddress,
		ClusterShardID: clientClusterShard,
		S2sProxyConfig: echoClientConfig,
	}

	runSendRecvBench(b, echoServerInfo, echoClientInfo, payloadSize)
}

func runSendRecvBench(b *testing.B, echoServerInfo endtoendtest.ClusterInfo, echoClientInfo endtoendtest.ClusterInfo, payloadSize int) {
	logger := log.NewTestLogger()
	echoServer := endtoendtest.NewEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := endtoendtest.NewEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoServer.SetPayloadSize(payloadSize)

	echoClient.Start()
	echoServer.Start()

	defer func() {
		echoClient.Stop()
		echoServer.Stop()
	}()

	streamClient, err := echoClient.CreateStreamClient()
	if err != nil {
		b.Fatalf("err: %v", err)
	}

	defer func() {
		_ = streamClient.CloseSend()
	}()

	b.ReportAllocs()
	b.ResetTimer()

	errCh := make(chan error, 1)
	go func() {
		for i := 0; i < b.N; i++ {
			highWatermarkInfo := &endtoendtest.WatermarkInfo{
				Watermark: int64(i),
			}

			req := &adminservice.StreamWorkflowReplicationMessagesRequest{
				Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
					SyncReplicationState: &replicationpb.SyncReplicationState{
						HighPriorityState: &replicationpb.ReplicationState{
							InclusiveLowWatermark:     highWatermarkInfo.Watermark,
							InclusiveLowWatermarkTime: timestamppb.New(highWatermarkInfo.Timestamp),
						},
					},
				}}

			if err = streamClient.Send(req); err != nil {
				errCh <- err
				return
			}
		}

		errCh <- nil
	}()

	for i := 0; i < b.N; i++ {
		_, err := streamClient.Recv()
		if err != nil {
			b.Fatalf("err: %v", err)
		}
	}

	<-errCh
}

func BenchmarkStreamSendRecvWithoutProxy_1k(b *testing.B) {
	benchmarkStreamSendRecvWithoutProxy(b, 1024)
}

func BenchmarkStreamSendRecvWithMuxProxy_1K(b *testing.B) {
	benchmarkStreamSendRecvWithMuxProxy(b, 1024)
}
