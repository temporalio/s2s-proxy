package proxy

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"
)

const (
	echoServerAddress          = "localhost:7266"
	serverProxyInboundAddress  = "localhost:7366"
	serverProxyOutboundAddress = "localhost:7466"
	prometheusAddress          = "localhost:7467"
	healthCheckAddress         = "localhost:7478"
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

	emptyReq = &adminservice.StreamWorkflowReplicationMessagesRequest{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
			SyncReplicationState: &replicationpb.SyncReplicationState{
				HighPriorityState: &replicationpb.ReplicationState{},
			},
		}}
)

type (
	proxyTestSuite struct {
		suite.Suite
		originalPath string
		developPath  string
	}

	cfgOption func(c *config.S2SProxyConfig)
)

func withServerTLS(tls encryption.ServerTLSConfig, inbound bool) cfgOption {
	return func(c *config.S2SProxyConfig) {
		if inbound {
			c.Inbound.Server.TLS = tls
		} else {
			c.Outbound.Server.TLS = tls
		}
	}
}

func withClientTLS(tls encryption.ClientTLSConfig, inbound bool) cfgOption {
	return func(c *config.S2SProxyConfig) {
		if inbound {
			c.Inbound.Client.TLS = tls
		} else {
			c.Outbound.Client.TLS = tls
		}
	}
}

func withACLPolicy(aclPolicy *config.ACLPolicy, inbound bool) cfgOption {
	return func(c *config.S2SProxyConfig) {
		if inbound {
			c.Inbound.ACLPolicy = aclPolicy
		} else {
			c.Outbound.ACLPolicy = aclPolicy
		}
	}
}

func withMux(mux config.MuxTransportConfig) cfgOption {
	return func(c *config.S2SProxyConfig) {
		c.MuxTransports = append(c.MuxTransports, mux)
	}
}

func withClientConfig(clientCfg config.ProxyClientConfig, inbound bool) cfgOption {
	return func(c *config.S2SProxyConfig) {
		if inbound {
			c.Inbound.Client = clientCfg
		} else {
			c.Outbound.Client = clientCfg
		}
	}
}

func withServerConfig(serverCfg config.ProxyServerConfig, inbound bool) cfgOption {
	return func(c *config.S2SProxyConfig) {
		if inbound {
			c.Inbound.Server = serverCfg
		} else {
			c.Outbound.Server = serverCfg
		}
	}
}

func withNamespaceTranslation(mapping []config.NameMappingConfig, _ bool) cfgOption {
	return func(c *config.S2SProxyConfig) {
		c.NamespaceNameTranslation.Mappings = mapping
	}
}

func EchoServerTLSOptions() []cfgOption {
	return []cfgOption{
		withServerTLS(
			encryption.ServerTLSConfig{
				CertificatePath:   filepath.Join("certificates", "proxy1.pem"),
				KeyPath:           filepath.Join("certificates", "proxy1.key"),
				ClientCAPath:      filepath.Join("certificates", "proxy2.pem"),
				RequireClientAuth: true,
			},
			true,
		),
		withClientTLS(
			encryption.ClientTLSConfig{
				CertificatePath: filepath.Join("certificates", "proxy1.pem"),
				KeyPath:         filepath.Join("certificates", "proxy1.key"),
				ServerName:      "onebox-proxy2.cluster.tmprl.cloud",
				ServerCAPath:    filepath.Join("certificates", "proxy2.pem"),
			},
			false,
		),
	}
}

func createS2SProxyConfig(cfg *config.S2SProxyConfig, opts []cfgOption) *config.S2SProxyConfig {
	for _, option := range opts {
		option(cfg)
	}

	return cfg
}

func createEchoServerConfig(opts ...cfgOption) *config.S2SProxyConfig {
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
		Metrics: &config.MetricsConfig{
			Prometheus: config.PrometheusConfig{
				ListenAddress: prometheusAddress,
				Framework:     "prometheus",
			},
		},
		HealthCheck: &config.HealthCheckConfig{
			Protocol:      "http",
			ListenAddress: healthCheckAddress,
		},
	}, opts)
}

func EchoClientTLSOptions() []cfgOption {
	return []cfgOption{
		withServerTLS(
			encryption.ServerTLSConfig{
				CertificatePath:   filepath.Join("certificates", "proxy2.pem"),
				KeyPath:           filepath.Join("certificates", "proxy2.key"),
				ClientCAPath:      filepath.Join("certificates", "proxy1.pem"),
				RequireClientAuth: true,
			},
			true,
		),
		withClientTLS(
			encryption.ClientTLSConfig{
				CertificatePath: filepath.Join("certificates", "proxy2.pem"),
				KeyPath:         filepath.Join("certificates", "proxy2.key"),
				ServerName:      "onebox-proxy1.cluster.tmprl.cloud",
				ServerCAPath:    filepath.Join("certificates", "proxy1.pem"),
			},
			false,
		),
	}
}

func createEchoClientConfig(opts ...cfgOption) *config.S2SProxyConfig {
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

func TestProxyTestSuite(t *testing.T) {
	suite.Run(t, new(proxyTestSuite))
}

func (s *proxyTestSuite) SetupTest() {
	var err error
	s.originalPath, err = os.Getwd()
	s.NoError(err)
	s.developPath = filepath.Join("..", "..", "develop")
	err = os.Chdir(s.developPath)
	s.NoError(err)
}

func (s *proxyTestSuite) TearDownTest() {
	err := os.Chdir(s.originalPath)
	s.NoError(err)
}

func (s *proxyTestSuite) SetupSubTest() {
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

// Run make generate-test-certs first befor running this test
func (s *proxyTestSuite) Test_Echo_Success() {
	tests := []struct {
		name           string
		echoServerInfo clusterInfo
		echoClientInfo clusterInfo
	}{
		{
			// echo_server <- - -> echo_client
			name: "no-proxy",
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
			// echo_server <-> proxy.inbound <- - -> echo_client
			name: "server-side-only-proxy",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: createEchoServerConfig(),
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
			},
		},
		{
			// echo_server <- - -> proxy.outbound <-> echo_client
			name: "client-side-only-proxy",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: createEchoClientConfig(),
			},
		},
		{
			// echo_server <-> proxy.inbound <- - -> proxy.outbound <-> echo_client
			name: "server-and-client-side-proxy",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: createEchoServerConfig(),
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: createEchoClientConfig(),
			},
		},
		{
			// echo_server <-> proxy.inbound <- mTLS -> proxy.outbound <-> echo_client
			name: "server-and-client-side-proxy-mTLS",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: createEchoServerConfig(EchoServerTLSOptions()...),
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: createEchoClientConfig(EchoClientTLSOptions()...),
			},
		},
		{
			name: "server-and-client-side-proxy-ACL",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: createEchoServerConfig(withACLPolicy(
					&config.ACLPolicy{
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
					true,
				)),
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: createEchoClientConfig(),
			},
		},
	}

	sequence := genSequence(1, 100)
	logger := log.NewTestLogger()
	for _, ts := range tests {
		echoServer := newEchoServer(ts.echoServerInfo, ts.echoClientInfo, "EchoServer", logger, nil)
		echoClient := newEchoServer(ts.echoClientInfo, ts.echoServerInfo, "EchoClient", logger, nil)
		echoServer.start()
		echoClient.start()

		s.Run(
			ts.name,
			func() {
				defer func() {
					echoClient.stop()
					echoServer.stop()
				}()

				// Test adminservice unary method
				r, err := retry(func() (*adminservice.DescribeClusterResponse, error) {
					return echoClient.DescribeCluster(&adminservice.DescribeClusterRequest{})
				}, 5, logger)
				s.NoError(err)
				s.Equal("EchoServer", r.ClusterName)

				// Test adminservice stream method
				echoed, err := echoClient.SendAndRecv(sequence)
				s.NoError(err)
				s.True(verifyEcho(sequence, echoed))

				// Test workflowservice
				resp, err := echoClient.PollActivityTaskQueue(&workflowservice.PollActivityTaskQueueRequest{
					Namespace: "example-ns",
				})
				s.NoError(err)
				s.Require().NotNil(resp)
				s.Equal("example-ns", resp.WorkflowNamespace)
				// Confirm that Prometheus initialized and is reporting. We should see proxy_start_count
				if proxyCfg := ts.echoServerInfo.s2sProxyConfig; proxyCfg != nil && proxyCfg.Metrics != nil {
					_, _ = http.Get("http://" + ts.echoServerInfo.s2sProxyConfig.HealthCheck.ListenAddress + "/health")
					logger.Info("Trying to check http://" + proxyCfg.Metrics.Prometheus.ListenAddress + "/metrics")
					metricsResp, err := http.Get("http://" + proxyCfg.Metrics.Prometheus.ListenAddress + "/metrics")
					s.NoError(err)
					metricsBytes, err := io.ReadAll(metricsResp.Body)
					s.NoError(err)
					metricsString := string(metricsBytes)
					s.Require().True(strings.Contains(metricsString, "proxy_start_count"),
						"metrics should contain proxy_start_count, but was \"%s\"", metricsString)
					s.Require().True(strings.Contains(metricsString, "proxy_health_check_success"),
						"metrics should contain proxy_health_check_success, but was \"%s\"", metricsString)
				}
			},
		)
	}
}

func (s *proxyTestSuite) Test_Echo_WithNamespaceTranslation() {
	tests := []struct {
		name            string
		echoServerInfo  clusterInfo
		echoClientInfo  clusterInfo
		serverNamespace string
		clientNamespace string
	}{
		{
			name: "server-and-client-side-proxy-namespacetrans",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: createEchoServerConfig(withNamespaceTranslation(
					[]config.NameMappingConfig{
						{
							LocalName:  "local",
							RemoteName: "remote",
						},
					},
					true,
				)),
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: createEchoClientConfig(),
			},
			serverNamespace: "local",
			clientNamespace: "remote",
		},
		{
			name: "server-and-client-side-proxy-namespacetrans-acl",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: createEchoServerConfig(
					withNamespaceTranslation(
						[]config.NameMappingConfig{
							{
								LocalName:  "local",
								RemoteName: "remote",
							},
						},
						true,
					),
					withACLPolicy(
						&config.ACLPolicy{
							AllowedMethods: config.AllowedMethods{
								AdminService: []string{
									"DescribeMutableState",
								},
							},
							AllowedNamespaces: []string{
								"local",
							},
						},
						true,
					),
				)},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: createEchoClientConfig(),
			},
			serverNamespace: "local",
			clientNamespace: "remote",
		},
	}

	logger := log.NewTestLogger()
	for _, ts := range tests {
		echoServer := newEchoServer(ts.echoServerInfo, ts.echoClientInfo, "EchoServer", logger, []string{ts.serverNamespace})
		echoClient := newEchoServer(ts.echoClientInfo, ts.echoServerInfo, "EchoClient", logger, nil)
		echoServer.start()
		echoClient.start()

		s.Run(
			ts.name,
			func() {
				defer func() {
					echoClient.stop()
					echoServer.stop()
				}()

				resp, err := retry(func() (*adminservice.DescribeMutableStateResponse, error) {
					return echoClient.DescribeMutableState(&adminservice.DescribeMutableStateRequest{
						Namespace: ts.clientNamespace,
					})
				}, 5, logger)
				s.NoError(err)
				s.Require().NotNil(resp)
			},
		)
	}
}

func (s *proxyTestSuite) Test_Echo_WithMuxTransport() {
	muxTransportName := "muxed"

	// Mux Transport
	//    echoServer muxClient(proxy1) -> muxServer(proxy2) echoClient
	//
	// echoServer proxy1.inbound.Server(muxClient)  <- proxy2.outbound.Client(muxServer) echoClient
	// echoServer proxy1.outbound.Client(muxClient) -> proxy2.inbound.Server(muxServer) echoClient
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

	logger := log.NewTestLogger()
	echoServer := newEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := newEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoClient.start()
	echoServer.start()

	defer func() {
		echoClient.stop()
		echoServer.stop()
	}()

	r, err := retry(func() (*adminservice.DescribeClusterResponse, error) {
		return echoClient.DescribeCluster(&adminservice.DescribeClusterRequest{})
	}, 5, logger)

	s.NoError(err)
	s.Equal("EchoServer", r.ClusterName)
}

func (s *proxyTestSuite) Test_ForceStopSourceServer() {
	logger := log.NewTestLogger()

	echoServerInfo := clusterInfo{
		serverAddress:  echoServerAddress,
		clusterShardID: serverClusterShard,
	}

	echoClientInfo := clusterInfo{
		serverAddress:  echoClientAddress,
		clusterShardID: clientClusterShard,
		s2sProxyConfig: createEchoClientConfig(),
	}

	echoServer := newEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := newEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoServer.start()
	echoClient.start()

	stream, err := echoClient.CreateStreamClient()
	s.NoError(err)
	_, err = sendRecv(stream, []int64{1})
	s.NoError(err)

	echoServer.server.ForceStop()

	// ForceStop cause sourceStreamClient.Recv in Upstream loop within
	// StreamWorkflowReplicationMessages handler to fail. Wait for
	// StreamWorkflowReplicationMessages handler returns, which stop
	// Downstream loop.
	time.Sleep(time.Second)

	err = stream.Send(emptyReq)

	// This should fail because StreamWorkflowReplicationMessages handler stopped.
	s.ErrorContains(err, "EOF")

	stream.CloseSend()
	echoClient.stop()
}
