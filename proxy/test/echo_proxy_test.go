package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
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

	serverProxyConfig = config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: "proxy1-inbound-server",
			Server: config.ServerConfig{
				ListenAddress: serverProxyInboundAddress,
			},
			Client: config.ClientConfig{
				ForwardAddress: echoServerAddress,
			},
		},
		Outbound: &config.ProxyConfig{
			Name: "proxy1-outbound-server",
			Server: config.ServerConfig{
				ListenAddress: serverProxyOutboundAddress,
			},
			Client: config.ClientConfig{
				ForwardAddress: "to-be-added",
			},
		},
	}

	clientProxyConfig = config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: "proxy2-inbound-server",
			Server: config.ServerConfig{
				ListenAddress: clientProxyInboundAddress,
			},
			Client: config.ClientConfig{
				ForwardAddress: echoClientAddress,
			},
		},
		Outbound: &config.ProxyConfig{
			Name: "proxy2-outbound-server",
			Server: config.ServerConfig{
				ListenAddress: clientProxyOutboundAddress,
			},
			Client: config.ClientConfig{
				ForwardAddress: "to-be-added",
			},
		},
	}

	serverProxyConfigWithTLS = config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: "proxy1-inbound-server",
			Server: config.ServerConfig{
				ListenAddress: serverProxyInboundAddress,
				TLS: encryption.ServerTLSConfig{
					CertificatePath:   filepath.Join("certificates", "proxy1.pem"),
					KeyPath:           filepath.Join("certificates", "proxy1.key"),
					ClientCAPath:      filepath.Join("certificates", "proxy2.pem"),
					RequireClientAuth: true,
				},
			},
			Client: config.ClientConfig{
				ForwardAddress: echoServerAddress,
			},
		},
		Outbound: &config.ProxyConfig{
			Name: "proxy1-outbound-server",
			Server: config.ServerConfig{
				ListenAddress: serverProxyOutboundAddress,
			},
			Client: config.ClientConfig{
				ForwardAddress: "to-be-added",
				TLS: encryption.ClientTLSConfig{
					CertificatePath: filepath.Join("certificates", "proxy1.pem"),
					KeyPath:         filepath.Join("certificates", "proxy1.key"),
					ServerName:      "onebox-proxy2.cluster.tmprl.cloud",
					ServerCAPath:    filepath.Join("certificates", "proxy2.pem"),
				},
			},
		},
	}

	clientProxyConfigWithTLS = config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: "proxy2-inbound-server",
			Server: config.ServerConfig{
				ListenAddress: clientProxyInboundAddress,
				TLS: encryption.ServerTLSConfig{
					CertificatePath:   filepath.Join("certificates", "proxy2.pem"),
					KeyPath:           filepath.Join("certificates", "proxy2.key"),
					ClientCAPath:      filepath.Join("certificates", "proxy1.pem"),
					RequireClientAuth: true,
				},
			},
			Client: config.ClientConfig{
				ForwardAddress: echoClientAddress,
			},
		},
		Outbound: &config.ProxyConfig{
			Name: "proxy2-outbound-server",
			Server: config.ServerConfig{
				ListenAddress: clientProxyOutboundAddress,
			},
			Client: config.ClientConfig{
				ForwardAddress: "to-be-added",
				TLS: encryption.ClientTLSConfig{
					CertificatePath: filepath.Join("certificates", "proxy2.pem"),
					KeyPath:         filepath.Join("certificates", "proxy2.key"),
					ServerName:      "onebox-proxy1.cluster.tmprl.cloud",
					ServerCAPath:    filepath.Join("certificates", "proxy1.pem"),
				},
			},
		},
	}

	serverProxyConfigWithACL = config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: "proxy1-inbound-server",
			Server: config.ServerConfig{
				ListenAddress: serverProxyInboundAddress,
			},
			Client: config.ClientConfig{
				ForwardAddress: echoServerAddress,
			},
			ACLPolicy: &config.ACLPolicy{
				AllowedMethods: config.AllowedMethods{
					AdminService: []string{
						"DescribeCluster",
						"StreamWorkflowReplicationMessages",
					},
				},
			},
		},
		Outbound: &config.ProxyConfig{
			Name: "proxy1-outbound-server",
			Server: config.ServerConfig{
				ListenAddress: serverProxyOutboundAddress,
			},
			Client: config.ClientConfig{
				ForwardAddress: "to-be-added",
			},
		},
	}
)

type (
	proxyTestSuite struct {
		suite.Suite
		originalPath string
		developPath  string
	}
)

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
				s2sProxyConfig: &serverProxyConfig,
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
				s2sProxyConfig: &clientProxyConfig,
			},
		},
		{
			// echo_server <-> proxy.inbound <- - -> proxy.outbound <-> echo_client
			name: "server-and-client-side-proxy",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: &serverProxyConfig,
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: &clientProxyConfig,
			},
		},
		{
			// echo_server <-> proxy.inbound <- mTLS -> proxy.outbound <-> echo_client
			name: "server-and-client-side-proxy-mTLS",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: &serverProxyConfigWithTLS,
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: &clientProxyConfigWithTLS,
			},
		},
		{
			name: "server-and-client-side-proxy-ACL",
			echoServerInfo: clusterInfo{
				serverAddress:  echoServerAddress,
				clusterShardID: serverClusterShard,
				s2sProxyConfig: &serverProxyConfigWithACL,
			},
			echoClientInfo: clusterInfo{
				serverAddress:  echoClientAddress,
				clusterShardID: clientClusterShard,
				s2sProxyConfig: &clientProxyConfig,
			},
		},
	}

	sequence := genSequence(1, 100)
	logger := log.NewTestLogger()
	for _, ts := range tests {
		echoServer := newEchoServer(ts.echoServerInfo, ts.echoClientInfo, "EchoServer", logger)
		echoClient := newEchoServer(ts.echoClientInfo, ts.echoServerInfo, "EchoClient", logger)
		echoServer.start()
		echoClient.start()

		s.Run(
			ts.name,
			func() {
				defer func() {
					echoClient.stop()
					echoServer.stop()
				}()

				r, err := echoClient.DescribeCluster(&adminservice.DescribeClusterRequest{})
				s.NoError(err)
				s.Equal("EchoServer", r.ClusterName)

				// Test adminservice
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
