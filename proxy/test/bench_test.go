package proxy

import (
	"testing"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func benchmarkStreamSendRecvWithoutProxy(b *testing.B, payloadSize int) {

	echoServerInfo := clusterInfo{
		serverAddress:  echoServerAddress,
		clusterShardID: serverClusterShard,
	}

	echoClientInfo := clusterInfo{
		serverAddress:  echoClientAddress,
		clusterShardID: clientClusterShard,
	}

	runSendRecvBench(b, echoServerInfo, echoClientInfo, payloadSize)
}

func benchmarkStreamSendRecvWithMuxProxy(b *testing.B, payloadSize int) {
	b.Log("Start BenchmarkStreamSendRecv")
	muxTransportName := "muxed"

	echoServerConfig := createEchoServerConfig(
		withMux(
			config.MuxTransportConfig{
				Name: muxTransportName,
				Mode: config.ClientMode,
				Client: config.TCPClientSetting{
					ServerAddress: clientProxyInboundAddress,
				},
			}),
		withServerConfig(
			// proxy1.inbound.Server
			config.ProxyServerConfig{
				Type:             config.MuxTransport,
				MuxTransportName: muxTransportName,
			}, true),
		withClientConfig(
			// proxy1.outbound.Client
			config.ProxyClientConfig{
				Type:             config.MuxTransport,
				MuxTransportName: muxTransportName,
			}, false),
	)

	echoClientConfig := createEchoClientConfig(
		withMux(
			config.MuxTransportConfig{
				Name: muxTransportName,
				Mode: config.ServerMode,
				Server: config.TCPServerSetting{
					ListenAddress: clientProxyInboundAddress,
				},
			}),
		withServerConfig(
			// proxy2.inbound.Server
			config.ProxyServerConfig{
				Type:             config.MuxTransport,
				MuxTransportName: muxTransportName,
			}, true),
		withClientConfig(
			// proxy2.outbound.Client
			config.ProxyClientConfig{
				Type:             config.MuxTransport,
				MuxTransportName: muxTransportName,
			}, false),
	)

	echoServerInfo := clusterInfo{
		serverAddress:  echoServerAddress,
		clusterShardID: serverClusterShard,
		s2sProxyConfig: echoServerConfig,
	}
	echoClientInfo := clusterInfo{
		serverAddress:  echoClientAddress,
		clusterShardID: clientClusterShard,
		s2sProxyConfig: echoClientConfig,
	}

	runSendRecvBench(b, echoServerInfo, echoClientInfo, payloadSize)
}

func runSendRecvBench(b *testing.B, echoServerInfo clusterInfo, echoClientInfo clusterInfo, payloadSize int) {
	logger := log.NewTestLogger()
	echoServer := newEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := newEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoServer.setPayloadSize(payloadSize)

	echoClient.start()
	echoServer.start()

	defer func() {
		echoClient.stop()
		echoServer.stop()
	}()

	streamClient, err := echoClient.CreateStreamClient()
	if err != nil {
		b.Fatalf("err: %v", err)
	}

	defer streamClient.CloseSend()

	b.ReportAllocs()
	b.ResetTimer()

	errCh := make(chan error, 1)
	go func() {
		for i := 0; i < b.N; i++ {
			highWatermarkInfo := &watermarkInfo{
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
