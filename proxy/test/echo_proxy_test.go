package proxy

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/client/history"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/endtoendtest"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

func init() {
	// silence info log spam
	_ = os.Setenv("TEMPORAL_TEST_LOG_LEVEL", "error")
	mux.MuxManagerStartDelay = 0
}

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
		originalPath               string
		developPath                string
		echoServerAddress          string
		serverProxyInboundAddress  string
		serverProxyOutboundAddress string
		echoClientAddress          string
		clientProxyInboundAddress  string
		clientProxyOutboundAddress string
	}

	cfgOption func(c *config.S2SProxyConfig)
)

func conn(c *config.S2SProxyConfig) *config.ClusterConnConfig {
	return &c.ClusterConnections[0]
}

// withRemoteServerTLS configures TLS on the remote-facing server listener (the proxy's "inbound" entry point).
func withRemoteServerTLS(tls encryption.TLSConfig) cfgOption {
	return func(c *config.S2SProxyConfig) { conn(c).Remote.TcpServer.TLSConfig = tls }
}

// withRemoteClientTLS configures TLS on the remote-facing client (forwarding to the remote proxy).
func withRemoteClientTLS(tls encryption.TLSConfig) cfgOption {
	return func(c *config.S2SProxyConfig) { conn(c).Remote.TcpClient.TLSConfig = tls }
}

func withACLPolicy(aclPolicy *config.ACLPolicy) cfgOption {
	return func(c *config.S2SProxyConfig) { conn(c).ACLPolicy = aclPolicy }
}

func withRemoteMuxClient(address string) cfgOption {
	return func(c *config.S2SProxyConfig) {
		conn(c).Remote.ConnectionType = config.ConnTypeMuxClient
		conn(c).Remote.MuxAddressInfo.ConnectionString = address
	}
}

func withRemoteMuxServer(address string) cfgOption {
	return func(c *config.S2SProxyConfig) {
		conn(c).Remote.ConnectionType = config.ConnTypeMuxServer
		conn(c).Remote.MuxAddressInfo.ConnectionString = address
	}
}

func withNamespaceTranslation(mappings []config.StringMapping) cfgOption {
	return func(c *config.S2SProxyConfig) {
		conn(c).NamespaceTranslation.Mappings = mappings
	}
}

func EchoServerTLSOptions() []cfgOption {
	return []cfgOption{
		withRemoteServerTLS(encryption.TLSConfig{
			CertificatePath:    filepath.Join("certificates", "proxy1.pem"),
			KeyPath:            filepath.Join("certificates", "proxy1.key"),
			RemoteCAPath:       filepath.Join("certificates", "proxy2.pem"),
			SkipCAVerification: false,
		}),
		withRemoteClientTLS(encryption.TLSConfig{
			CertificatePath: filepath.Join("certificates", "proxy1.pem"),
			KeyPath:         filepath.Join("certificates", "proxy1.key"),
			CAServerName:    "onebox-proxy2.cluster.tmprl.cloud",
			RemoteCAPath:    filepath.Join("certificates", "proxy2.pem"),
		}),
	}
}

func createS2SProxyConfig(cfg *config.S2SProxyConfig, opts []cfgOption) *config.S2SProxyConfig {
	for _, option := range opts {
		option(cfg)
	}

	return cfg
}

func (s *proxyTestSuite) createEchoServerConfig(opts ...cfgOption) *config.S2SProxyConfig {
	return createS2SProxyConfig(&config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{{
			Name: "proxy1",
			Local: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpServer:      config.TCPTLSInfo{ConnectionString: s.serverProxyOutboundAddress},
				TcpClient:      config.TCPTLSInfo{ConnectionString: s.echoServerAddress},
			},
			Remote: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpServer:      config.TCPTLSInfo{ConnectionString: s.serverProxyInboundAddress},
				TcpClient:      config.TCPTLSInfo{ConnectionString: "to-be-added"},
			},
		}},
	}, opts)
}

func EchoClientTLSOptions() []cfgOption {
	return []cfgOption{
		withRemoteServerTLS(encryption.TLSConfig{
			CertificatePath:    filepath.Join("certificates", "proxy2.pem"),
			KeyPath:            filepath.Join("certificates", "proxy2.key"),
			RemoteCAPath:       filepath.Join("certificates", "proxy1.pem"),
			SkipCAVerification: false,
		}),
		withRemoteClientTLS(encryption.TLSConfig{
			CertificatePath: filepath.Join("certificates", "proxy2.pem"),
			KeyPath:         filepath.Join("certificates", "proxy2.key"),
			CAServerName:    "onebox-proxy1.cluster.tmprl.cloud",
			RemoteCAPath:    filepath.Join("certificates", "proxy1.pem"),
		}),
	}
}

func (s *proxyTestSuite) createEchoClientConfig(opts ...cfgOption) *config.S2SProxyConfig {
	return createS2SProxyConfig(&config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{{
			Name: "proxy2",
			Local: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpServer:      config.TCPTLSInfo{ConnectionString: s.clientProxyOutboundAddress},
				TcpClient:      config.TCPTLSInfo{ConnectionString: s.echoClientAddress},
			},
			Remote: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				TcpServer:      config.TCPTLSInfo{ConnectionString: s.clientProxyInboundAddress},
				TcpClient:      config.TCPTLSInfo{ConnectionString: "to-be-added"},
			},
		}},
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

	// Allocate free ports for each test
	s.echoServerAddress = GetLocalhostAddress()
	s.serverProxyInboundAddress = GetLocalhostAddress()
	s.serverProxyOutboundAddress = GetLocalhostAddress()
	s.echoClientAddress = GetLocalhostAddress()
	s.clientProxyInboundAddress = GetLocalhostAddress()
	s.clientProxyOutboundAddress = GetLocalhostAddress()
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
		echoServerInfo endtoendtest.ClusterInfo
		echoClientInfo endtoendtest.ClusterInfo
	}{
		{
			// echo_server <- - -> echo_client
			name: "no-proxy",
			echoServerInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoServerAddress,
				ClusterShardID: serverClusterShard,
			},
			echoClientInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoClientAddress,
				ClusterShardID: clientClusterShard,
			},
		},
		{
			// echo_server <-> proxy.inbound <- - -> echo_client
			name: "server-side-only-proxy",
			echoServerInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoServerAddress,
				ClusterShardID: serverClusterShard,
				S2sProxyConfig: s.createEchoServerConfig(),
			},
			echoClientInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoClientAddress,
				ClusterShardID: clientClusterShard,
			},
		},
		{
			// echo_server <- - -> proxy.outbound <-> echo_client
			name: "client-side-only-proxy",
			echoServerInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoServerAddress,
				ClusterShardID: serverClusterShard,
			},
			echoClientInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoClientAddress,
				ClusterShardID: clientClusterShard,
				S2sProxyConfig: s.createEchoClientConfig(),
			},
		},
		{
			// echo_server <-> proxy.inbound <- - -> proxy.outbound <-> echo_client
			name: "server-and-client-side-proxy",
			echoServerInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoServerAddress,
				ClusterShardID: serverClusterShard,
				S2sProxyConfig: s.createEchoServerConfig(),
			},
			echoClientInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoClientAddress,
				ClusterShardID: clientClusterShard,
				S2sProxyConfig: s.createEchoClientConfig(),
			},
		},
		{
			// echo_server <-> proxy.inbound <- mTLS -> proxy.outbound <-> echo_client
			name: "server-and-client-side-proxy-mTLS",
			echoServerInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoServerAddress,
				ClusterShardID: serverClusterShard,
				S2sProxyConfig: s.createEchoServerConfig(EchoServerTLSOptions()...),
			},
			echoClientInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoClientAddress,
				ClusterShardID: clientClusterShard,
				S2sProxyConfig: s.createEchoClientConfig(EchoClientTLSOptions()...),
			},
		},
		{
			name: "server-and-client-side-proxy-ACL",
			echoServerInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoServerAddress,
				ClusterShardID: serverClusterShard,
				S2sProxyConfig: s.createEchoServerConfig(withACLPolicy(
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
				)),
			},
			echoClientInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoClientAddress,
				ClusterShardID: clientClusterShard,
				S2sProxyConfig: s.createEchoClientConfig(),
			},
		},
	}

	sequence := genSequence(1, 100)
	logger := log.NewTestLogger()
	for _, ts := range tests {
		s.Run(
			ts.name,
			func() {
				echoServer := endtoendtest.NewEchoServer(ts.echoServerInfo, ts.echoClientInfo, "EchoServer", logger, nil)
				echoClient := endtoendtest.NewEchoServer(ts.echoClientInfo, ts.echoServerInfo, "EchoClient", logger, nil)
				echoServer.Start()
				echoClient.Start()
				defer func() {
					echoClient.Stop()
					echoServer.Stop()
				}()
				// Test adminservice unary method
				r, err := endtoendtest.Retry(func() (*adminservice.DescribeClusterResponse, error) {
					return echoClient.DescribeCluster(&adminservice.DescribeClusterRequest{})
				}, 5, logger)
				require.NoErrorf(s.T(), err, "Couldn't describeCluster!\nserver:%s\nclient:%s", echoServer.Describe(), echoClient.Describe())
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
			},
		)
	}
}

func (s *proxyTestSuite) Test_Echo_WithNamespaceTranslation() {
	tests := []struct {
		name            string
		echoServerInfo  endtoendtest.ClusterInfo
		echoClientInfo  endtoendtest.ClusterInfo
		serverNamespace string
		clientNamespace string
	}{
		{
			name: "server-and-client-side-proxy-namespacetrans",
			echoServerInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoServerAddress,
				ClusterShardID: serverClusterShard,
				S2sProxyConfig: s.createEchoServerConfig(withNamespaceTranslation(
					[]config.StringMapping{
						{Local: "local", Remote: "remote"},
					},
				)),
			},
			echoClientInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoClientAddress,
				ClusterShardID: clientClusterShard,
				S2sProxyConfig: s.createEchoClientConfig(),
			},
			serverNamespace: "local",
			clientNamespace: "remote",
		},
		{
			name: "server-and-client-side-proxy-namespacetrans-acl",
			echoServerInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoServerAddress,
				ClusterShardID: serverClusterShard,
				S2sProxyConfig: s.createEchoServerConfig(
					withNamespaceTranslation(
						[]config.StringMapping{
							{Local: "local", Remote: "remote"},
						},
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
					),
				)},
			echoClientInfo: endtoendtest.ClusterInfo{
				ServerAddress:  s.echoClientAddress,
				ClusterShardID: clientClusterShard,
				S2sProxyConfig: s.createEchoClientConfig(),
			},
			serverNamespace: "local",
			clientNamespace: "remote",
		},
	}

	logger := log.NewTestLogger()
	for _, ts := range tests {
		echoServer := endtoendtest.NewEchoServer(ts.echoServerInfo, ts.echoClientInfo, "EchoServer", logger, []string{ts.serverNamespace})
		echoClient := endtoendtest.NewEchoServer(ts.echoClientInfo, ts.echoServerInfo, "EchoClient", logger, nil)
		echoServer.Start()
		echoClient.Start()

		s.Run(
			ts.name,
			func() {
				defer func() {
					echoClient.Stop()
					echoServer.Stop()
				}()

				resp, err := endtoendtest.Retry(func() (*adminservice.DescribeMutableStateResponse, error) {
					return echoClient.DescribeMutableState(&adminservice.DescribeMutableStateRequest{
						Namespace: ts.clientNamespace,
					})
				}, 5, logger)
				s.Require().NoError(err)
				s.Require().NotNil(resp)
			},
		)
	}
}

func (s *proxyTestSuite) Test_Echo_WithMuxTransport() {
	// Mux Transport
	//    echoServer muxClient(proxy1) -> muxServer(proxy2) echoClient
	//
	// proxy1 dials proxy2's mux listener at clientProxyInboundAddress;
	// the established connection multiplexes both directions.
	echoServerConfig := s.createEchoServerConfig(
		withRemoteMuxClient(s.clientProxyInboundAddress),
	)

	echoClientConfig := s.createEchoClientConfig(
		withRemoteMuxServer(s.clientProxyInboundAddress),
	)

	echoServerInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoServerAddress,
		ClusterShardID: serverClusterShard,
		S2sProxyConfig: echoServerConfig,
	}
	echoClientInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoClientAddress,
		ClusterShardID: clientClusterShard,
		S2sProxyConfig: echoClientConfig,
	}

	logger := log.NewTestLogger()
	echoServer := endtoendtest.NewEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := endtoendtest.NewEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoClient.Start()
	echoServer.Start()

	defer func() {
		echoClient.Stop()
		echoServer.Stop()
	}()

	r, err := endtoendtest.Retry(func() (*adminservice.DescribeClusterResponse, error) {
		return echoClient.DescribeCluster(&adminservice.DescribeClusterRequest{})
	}, 5, logger)

	require.NoErrorf(s.T(), err, "Should have received a response from echo server!\nserver:%s\nclient:%s", echoServer.Describe(), echoClient.Describe())
	s.Equal("EchoServer", r.ClusterName)
}

func (s *proxyTestSuite) Test_ForceStopSourceServer() {
	logger := log.NewTestLogger()

	echoServerInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoServerAddress,
		ClusterShardID: serverClusterShard,
	}

	echoClientInfo := endtoendtest.ClusterInfo{
		ServerAddress:  s.echoClientAddress,
		ClusterShardID: clientClusterShard,
		S2sProxyConfig: s.createEchoClientConfig(),
	}

	echoServer := endtoendtest.NewEchoServer(echoServerInfo, echoClientInfo, "EchoServer", logger, nil)
	echoClient := endtoendtest.NewEchoServer(echoClientInfo, echoServerInfo, "EchoClient", logger, nil)

	echoServer.Start()
	echoClient.Start()
	defer func() {
		echoClient.Stop()
		echoServer.Stop()
	}()

	stream, err := echoClient.CreateStreamClient()
	s.NoError(err)
	_, err = endtoendtest.SendRecv(stream, []int64{1})
	s.NoError(err)

	echoServer.Temporal.ForceStop()

	// ForceStop cause sourceStreamClient.Recv in Upstream loop within
	// StreamWorkflowReplicationMessages handler to fail. Wait for
	// StreamWorkflowReplicationMessages handler returns, which stop
	// Downstream loop.
	time.Sleep(time.Second)

	err = stream.Send(emptyReq)

	// This should fail because StreamWorkflowReplicationMessages handler stopped.
	s.ErrorContains(err, "EOF")

	_ = stream.CloseSend()
}
