package proxy

import (
	"fmt"
	"testing"

	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/endtoendtest"
	"github.com/temporalio/s2s-proxy/testutil"
)

func createEchoServerConfigWithPorts(
	echoServerAddress string,
	serverProxyInboundAddress string,
	serverProxyOutboundAddress string,
	opts ...cfgOption,
) *config.S2SProxyConfig {
	return createS2SProxyConfig(&config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: "proxy1-inbound-server",
			Server: config.ProxyServerConfig{
				TCPServerSetting: config.TCPServerSetting{
					ListenAddress: serverProxyInboundAddress,
				},
			},
			Client: config.ProxyClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: echoServerAddress,
				},
			},
		},
		Outbound: &config.ProxyConfig{
			Name: "proxy1-outbound-server",
			Server: config.ProxyServerConfig{
				TCPServerSetting: config.TCPServerSetting{
					ListenAddress: serverProxyOutboundAddress,
				},
			},
			Client: config.ProxyClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: "to-be-added",
				},
			},
		},
	}, opts)
}

func createEchoClientConfigWithPorts(
	echoClientAddress string,
	clientProxyInboundAddress string,
	clientProxyOutboundAddress string,
	opts ...cfgOption,
) *config.S2SProxyConfig {
	return createS2SProxyConfig(&config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: "proxy2-inbound-server",
			Server: config.ProxyServerConfig{
				TCPServerSetting: config.TCPServerSetting{
					ListenAddress: clientProxyInboundAddress,
				},
			},
			Client: config.ProxyClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: echoClientAddress,
				},
			},
		},
		Outbound: &config.ProxyConfig{
			Name: "proxy2-outbound-server",
			Server: config.ProxyServerConfig{
				TCPServerSetting: config.TCPServerSetting{
					ListenAddress: clientProxyOutboundAddress,
				},
			},
			Client: config.ProxyClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: "to-be-added",
				},
			},
		},
	}, opts)
}

func benchmarkStreamSendRecvWithoutProxy(b *testing.B, payloadSize int) {
	echoServerInfo := endtoendtest.ClusterInfo{
		ServerAddress:  fmt.Sprintf("localhost:%d", testutil.GetFreePort()),
		ClusterShardID: serverClusterShard,
	}

	echoClientInfo := endtoendtest.ClusterInfo{
		ServerAddress:  fmt.Sprintf("localhost:%d", testutil.GetFreePort()),
		ClusterShardID: clientClusterShard,
	}

	runSendRecvBench(b, echoServerInfo, echoClientInfo, payloadSize)
}

func benchmarkStreamSendRecvWithMuxProxy(b *testing.B, payloadSize int) {
	b.Log("Start BenchmarkStreamSendRecv")
	muxTransportName := "muxed"

	// Allocate ports dynamically
	echoServerAddress := fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	serverProxyInboundAddress := fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	serverProxyOutboundAddress := fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	echoClientAddress := fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	clientProxyInboundAddress := fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	clientProxyOutboundAddress := fmt.Sprintf("localhost:%d", testutil.GetFreePort())

	echoServerConfig := createEchoServerConfigWithPorts(
		echoServerAddress,
		serverProxyInboundAddress,
		serverProxyOutboundAddress,
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

	echoClientConfig := createEchoClientConfigWithPorts(
		echoClientAddress,
		clientProxyInboundAddress,
		clientProxyOutboundAddress,
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
