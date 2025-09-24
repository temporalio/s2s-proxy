package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gogo/status"
	"github.com/stretchr/testify/assert"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/testserver"
)

type (
	s2sAddresses struct {
		echoServer  string
		inbound     string
		outbound    string
		prometheus  string
		healthCheck string
	}
)

var (
	// Create some believable echo server configs
	echoServerInfo = testserver.ClusterInfo{
		ServerAddress:  echoServerAddress,
		ClusterShardID: serverClusterShard,
		S2sProxyConfig: makeS2SConfig(s2sAddresses{
			echoServer:  "localhost:7266",
			inbound:     "localhost:7366",
			outbound:    "localhost:7466",
			prometheus:  "localhost:7468",
			healthCheck: "localhost:7479",
		}),
	}
	echoClientInfo = testserver.ClusterInfo{
		ServerAddress:  echoClientAddress,
		ClusterShardID: clientClusterShard,
		S2sProxyConfig: makeS2SConfig(s2sAddresses{
			echoServer:  "localhost:8266",
			inbound:     "localhost:8366",
			outbound:    "localhost:8466",
			prometheus:  "localhost:7467",
			healthCheck: "localhost:7478",
		}),
	}
	logger log.Logger
)

func init() {
	_ = os.Setenv("TEMPORAL_TEST_LOG_LEVEL", "error")
	logger = log.NewTestLogger()
}

type hangupAdminServer struct {
	adminservice.UnimplementedAdminServiceServer
}

func (s *hangupAdminServer) StreamWorkflowReplicationMessages(server adminservice.AdminService_StreamWorkflowReplicationMessagesServer) error {
	go func() {
		for {
			_, err := server.Recv()
			if status.Code(err) == codes.Canceled {
				logger.Info("Client closed")
				return
			} else if err != nil {
				logger.Info("Got a message with error", tag.Error(err))
			}
		}
	}()
	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		err := server.Send(&adminservice.StreamWorkflowReplicationMessagesResponse{})
		if err != nil {
			logger.Info("Got a message with error", tag.Error(err))
		}
		err = server.Send(&adminservice.StreamWorkflowReplicationMessagesResponse{})
		if err != nil {
			logger.Info("Got a message with error", tag.Error(err))
		}
		wg.Done()
	}()
	wg.Wait()
	return nil
}

func TestEOFFromServer(t *testing.T) {
	adminHandler := &hangupAdminServer{}
	grpcHost := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(grpcHost, adminHandler)
	listener, _ := net.Listen("tcp", "localhost:8566")
	go func() {
		_ = grpcHost.Serve(listener)
	}()
	time.Sleep(10 * time.Millisecond)
	client, err := grpc.NewClient("localhost:8566", grpc.WithTransportCredentials(insecure.NewCredentials()))
	assert.NoError(t, err)
	adminServiceClient := adminservice.NewAdminServiceClient(client)
	clientCtx, cancelCtx := context.WithCancel(t.Context())
	streamServer, err := adminServiceClient.StreamWorkflowReplicationMessages(clientCtx)
	assert.NoError(t, err)
	err = streamServer.Send(&adminservice.StreamWorkflowReplicationMessagesRequest{})
	assert.NoError(t, err)
	_, err = streamServer.Recv()
	assert.NoError(t, err)
	_, err = streamServer.Recv()
	assert.NoError(t, err)
	_, err = streamServer.Recv()
	assert.True(t, err == io.EOF, "Should have thrown io.EOF, but got %v instead! Error() returned: %v", err, err.Error())
	grpcHost.Stop()
	_ = listener.Close()
	cancelCtx()
}

func TestWiringWithEchoService(t *testing.T) {
	echoServer := testserver.NewEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := testserver.NewEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)
	echoServer.Start()
	echoClient.Start()
	defer func() {
		echoClient.Stop()
		echoServer.Stop()
	}()
	// Test s2s-proxy health check

	// The server may take a few 10s of ms to start
	var healthErr = fmt.Errorf("Not started")
	for attempts := 0; healthErr != nil && attempts < 5; attempts++ {
		_, healthErr = http.Get(fmt.Sprintf("http://%s/health", echoServerInfo.S2sProxyConfig.HealthCheck.ListenAddress))
		time.Sleep(10 * time.Millisecond)
	}
	assert.NoError(t, healthErr)

	// Confirm that Prometheus initialized and is reporting. We should see proxy_start_count
	serverMetrics := scrapePrometheus(t, echoServerInfo.S2sProxyConfig.Metrics.Prometheus.ListenAddress)
	assert.Contains(t, serverMetrics, "proxy_start_count",
		"metrics should contain proxy_start_count, but was \"%s\"", serverMetrics)
	assert.Contains(t, serverMetrics, "proxy_health_check_success",
		"metrics should contain proxy_health_check_success, but was \"%s\"", serverMetrics)

	// Make some calls and check that the gRPC metrics are reporting
	r, err := testserver.Retry(func() (*adminservice.DescribeClusterResponse, error) {
		return echoClient.DescribeCluster(&adminservice.DescribeClusterRequest{})
	}, 5, logger)
	assert.NoError(t, err)
	assert.Equal(t, "EchoServer", r.ClusterName)

	// Test adminservice stream method
	echoed, err := echoClient.SendAndRecv([]int64{1, 2, 3})
	assert.NoError(t, err)
	assert.True(t, verifyEcho([]int64{1, 2, 3}, echoed))

	// Test workflowservice
	resp, err := echoClient.PollActivityTaskQueue(&workflowservice.PollActivityTaskQueueRequest{
		Namespace: "example-ns",
	})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "example-ns", resp.WorkflowNamespace)

	clientMetrics := scrapePrometheus(t, echoClientInfo.S2sProxyConfig.Metrics.Prometheus.ListenAddress)

	assert.Contains(t, clientMetrics, "temporal_s2s_proxy_grpc_server_handled_total",
		"grpc counter metrics are missing or not prefixed properly")
	assert.Contains(t, clientMetrics, "temporal_s2s_proxy_grpc_server_handling_seconds_bucket",
		"grpc histogram metrics are missing or not prefixed properly")
}

func scrapePrometheus(t *testing.T, address string) string {
	logger.Info(fmt.Sprintf("Trying to check http://%s/metrics", address))
	metricsResp, err := http.Get(fmt.Sprintf("http://%s/metrics", address))
	assert.NoError(t, err)
	metricsBytes, err := io.ReadAll(metricsResp.Body)
	assert.NoError(t, err)
	return string(metricsBytes)
}

func makeS2SConfig(addresses s2sAddresses) *config.S2SProxyConfig {
	return &config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: "proxy1-inbound-server",
			Server: config.ProxyServerConfig{
				TCPServerSetting: config.TCPServerSetting{
					ListenAddress: addresses.inbound,
				},
			},
			Client: config.ProxyClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: addresses.echoServer,
				},
			},
			ACLPolicy: &config.ACLPolicy{
				AllowedMethods: config.AllowedMethods{
					AdminService: []string{
						"DescribeCluster",
						"StreamWorkflowReplicationMessages",
					},
				},
				AllowedNamespaces: []string{
					"example-ns",
				},
			},
		},
		Outbound: &config.ProxyConfig{
			Name: "proxy1-outbound-server",
			Server: config.ProxyServerConfig{
				TCPServerSetting: config.TCPServerSetting{
					ListenAddress: addresses.outbound,
				},
			},
			Client: config.ProxyClientConfig{
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: "to-be-added",
				},
			},
		},
		Metrics: &config.MetricsConfig{
			Prometheus: config.PrometheusConfig{
				ListenAddress: addresses.prometheus,
				Framework:     "prometheus",
			},
		},
		HealthCheck: &config.HealthCheckConfig{
			Protocol:      "http",
			ListenAddress: addresses.healthCheck,
		},
	}
}
