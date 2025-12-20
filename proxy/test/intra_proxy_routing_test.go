package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/tests/testcore"

	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
	"github.com/temporalio/s2s-proxy/testutil"
)

type (
	IntraProxyRoutingTestSuite struct {
		suite.Suite
		*require.Assertions

		logger log.Logger

		clusterA *testcore.TestCluster
		clusterB *testcore.TestCluster

		proxyA1 *s2sproxy.Proxy
		proxyA2 *s2sproxy.Proxy
		proxyB1 *s2sproxy.Proxy
		proxyB2 *s2sproxy.Proxy

		proxyA1Outbound string
		proxyA2Outbound string
		proxyB1Outbound string
		proxyB2Outbound string

		proxyB1Mux string
		proxyB2Mux string

		proxyA1MemberlistPort int
		proxyA2MemberlistPort int
		proxyB1MemberlistPort int
		proxyB2MemberlistPort int

		loadBalancerA *trackingTCPProxy
		loadBalancerB *trackingTCPProxy
		loadBalancerC *trackingTCPProxy

		loadBalancerAPort string
		loadBalancerBPort string
		loadBalancerCPort string

		connectionCountsA1  atomic.Int64
		connectionCountsA2  atomic.Int64
		connectionCountsB1  atomic.Int64
		connectionCountsB2  atomic.Int64
		connectionCountsPA1 atomic.Int64
		connectionCountsPA2 atomic.Int64
	}
)

func TestIntraProxyRoutingTestSuite(t *testing.T) {
	s := &IntraProxyRoutingTestSuite{}
	suite.Run(t, s)
}

func (s *IntraProxyRoutingTestSuite) SetupSuite() {
	s.Assertions = require.New(s.T())
	s.logger = log.NewTestLogger()

	s.logger.Info("Setting up intra-proxy routing test suite")

	s.clusterA = s.createCluster("cluster-a", 2, 1, 1)
	s.clusterB = s.createCluster("cluster-b", 2, 2, 1)

	s.proxyA1Outbound = fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	s.proxyA2Outbound = fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	s.proxyB1Outbound = fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	s.proxyB2Outbound = fmt.Sprintf("localhost:%d", testutil.GetFreePort())

	s.proxyB1Mux = fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	s.proxyB2Mux = fmt.Sprintf("localhost:%d", testutil.GetFreePort())

	loadBalancerAPort := fmt.Sprintf("%d", testutil.GetFreePort())
	loadBalancerBPort := fmt.Sprintf("%d", testutil.GetFreePort())
	loadBalancerCPort := fmt.Sprintf("%d", testutil.GetFreePort())

	s.loadBalancerAPort = loadBalancerAPort
	s.loadBalancerBPort = loadBalancerBPort
	s.loadBalancerCPort = loadBalancerCPort

	proxyA1Address := fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	proxyA2Address := fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	proxyB1Address := fmt.Sprintf("localhost:%d", testutil.GetFreePort())
	proxyB2Address := fmt.Sprintf("localhost:%d", testutil.GetFreePort())

	proxyAddressesA := map[string]string{
		"proxy-node-a-1": proxyA1Address,
		"proxy-node-a-2": proxyA2Address,
	}
	proxyAddressesB := map[string]string{
		"proxy-node-b-1": proxyB1Address,
		"proxy-node-b-2": proxyB2Address,
	}

	s.proxyA1MemberlistPort = testutil.GetFreePort()
	s.proxyA2MemberlistPort = testutil.GetFreePort()
	s.proxyB1MemberlistPort = testutil.GetFreePort()
	s.proxyB2MemberlistPort = testutil.GetFreePort()

	s.proxyB1 = s.createProxy("proxy-b-1", proxyB1Address, s.proxyB1Outbound, s.proxyB1Mux, s.clusterB, config.ServerMode, config.ShardCountConfig{}, "proxy-node-b-1", "127.0.0.1", s.proxyB1MemberlistPort, nil, proxyAddressesB)
	s.proxyB2 = s.createProxy("proxy-b-2", proxyB2Address, s.proxyB2Outbound, s.proxyB2Mux, s.clusterB, config.ServerMode, config.ShardCountConfig{}, "proxy-node-b-2", "127.0.0.1", s.proxyB2MemberlistPort, []string{fmt.Sprintf("127.0.0.1:%d", s.proxyB1MemberlistPort)}, proxyAddressesB)

	s.logger.Info("Setting up load balancers")

	s.loadBalancerA = s.createLoadBalancer(loadBalancerAPort, []string{s.proxyA1Outbound, s.proxyA2Outbound}, &s.connectionCountsA1, &s.connectionCountsA2)
	s.loadBalancerB = s.createLoadBalancer(loadBalancerBPort, []string{s.proxyB1Mux, s.proxyB2Mux}, &s.connectionCountsPA1, &s.connectionCountsPA2)
	s.loadBalancerC = s.createLoadBalancer(loadBalancerCPort, []string{s.proxyB1Outbound, s.proxyB2Outbound}, &s.connectionCountsB1, &s.connectionCountsB2)

	muxLoadBalancerBAddress := fmt.Sprintf("localhost:%s", loadBalancerBPort)
	s.proxyA1 = s.createProxy("proxy-a-1", proxyA1Address, s.proxyA1Outbound, muxLoadBalancerBAddress, s.clusterA, config.ClientMode, config.ShardCountConfig{}, "proxy-node-a-1", "127.0.0.1", s.proxyA1MemberlistPort, nil, proxyAddressesA)
	s.proxyA2 = s.createProxy("proxy-a-2", proxyA2Address, s.proxyA2Outbound, muxLoadBalancerBAddress, s.clusterA, config.ClientMode, config.ShardCountConfig{}, "proxy-node-a-2", "127.0.0.1", s.proxyA2MemberlistPort, []string{fmt.Sprintf("127.0.0.1:%d", s.proxyA1MemberlistPort)}, proxyAddressesA)

	s.logger.Info("Waiting for proxies to start and connect")
	time.Sleep(15 * time.Second)

	s.logger.Info("Configuring remote clusters")
	s.configureRemoteCluster(s.clusterA, s.clusterB.ClusterName(), fmt.Sprintf("localhost:%s", loadBalancerAPort))
	s.configureRemoteCluster(s.clusterB, s.clusterA.ClusterName(), fmt.Sprintf("localhost:%s", loadBalancerCPort))
	s.waitForReplicationReady()
}

func (s *IntraProxyRoutingTestSuite) TearDownSuite() {
	s.logger.Info("Tearing down intra-proxy routing test suite")
	if s.clusterA != nil && s.clusterB != nil {
		s.logger.Info("Removing remote cluster A from cluster B")
		s.removeRemoteCluster(s.clusterA, s.clusterB.ClusterName())
		s.logger.Info("Remote cluster A removed")
		s.logger.Info("Removing remote cluster B from cluster A")
		s.removeRemoteCluster(s.clusterB, s.clusterA.ClusterName())
		s.logger.Info("Remote cluster B removed")
	}
	if s.clusterA != nil {
		s.NoError(s.clusterA.TearDownCluster())
		s.logger.Info("Cluster A torn down")
	}
	if s.clusterB != nil {
		s.NoError(s.clusterB.TearDownCluster())
		s.logger.Info("Cluster B torn down")
	}
	if s.loadBalancerA != nil {
		s.logger.Info("Stopping load balancer A")
		s.loadBalancerA.Stop()
		s.logger.Info("Load balancer A stopped")
	}
	if s.loadBalancerB != nil {
		s.logger.Info("Stopping load balancer B")
		s.loadBalancerB.Stop()
		s.logger.Info("Load balancer B stopped")
	}
	if s.loadBalancerC != nil {
		s.logger.Info("Stopping load balancer C")
		s.loadBalancerC.Stop()
		s.logger.Info("Load balancer C stopped")
	}
	if s.proxyA1 != nil {
		s.logger.Info("Stopping proxy A1")
		s.proxyA1.Stop()
		s.logger.Info("Proxy A1 stopped")
	}
	if s.proxyA2 != nil {
		s.logger.Info("Stopping proxy A2")
		s.proxyA2.Stop()
		s.logger.Info("Proxy A2 stopped")
	}
	if s.proxyB1 != nil {
		s.logger.Info("Stopping proxy B1")
		s.proxyB1.Stop()
		s.logger.Info("Proxy B1 stopped")
	}
	if s.proxyB2 != nil {
		s.logger.Info("Stopping proxy B2")
		s.proxyB2.Stop()
		s.logger.Info("Proxy B2 stopped")
	}
	s.logger.Info("Intra-proxy routing test suite torn down")
}

func (s *IntraProxyRoutingTestSuite) createCluster(
	clusterName string,
	numShards int,
	initialFailoverVersion int64,
	numHistoryHosts int,
) *testcore.TestCluster {
	clusterSuffix := common.GenerateRandomString(8)
	fullClusterName := fmt.Sprintf("%s-%s", clusterName, clusterSuffix)

	clusterConfig := &testcore.TestClusterConfig{
		ClusterMetadata: cluster.Config{
			EnableGlobalNamespace:    true,
			FailoverVersionIncrement: 10,
			MasterClusterName:        fullClusterName,
			CurrentClusterName:       fullClusterName,
			ClusterInformation: map[string]cluster.ClusterInformation{
				fullClusterName: {
					Enabled:                true,
					InitialFailoverVersion: initialFailoverVersion,
				},
			},
		},
		HistoryConfig: testcore.HistoryConfig{
			NumHistoryShards: int32(numShards),
			NumHistoryHosts:  numHistoryHosts,
		},
		DynamicConfigOverrides: map[dynamicconfig.Key]interface{}{
			dynamicconfig.NamespaceCacheRefreshInterval.Key(): time.Second,
			dynamicconfig.EnableReplicationStream.Key():       true,
			dynamicconfig.EnableReplicationTaskBatching.Key(): true,
		},
	}

	testClusterFactory := testcore.NewTestClusterFactory()
	logger := log.With(s.logger, tag.NewStringTag("clusterName", clusterName))
	cluster, err := testClusterFactory.NewCluster(s.T(), clusterConfig, logger)
	s.NoError(err, "Failed to create cluster %s", clusterName)
	s.NotNil(cluster)

	return cluster
}

func (s *IntraProxyRoutingTestSuite) createProxy(
	name string,
	inboundAddress string,
	outboundAddress string,
	muxAddress string,
	cluster *testcore.TestCluster,
	muxMode config.MuxMode,
	shardCountConfig config.ShardCountConfig,
	nodeName string,
	memberlistBindAddr string,
	memberlistBindPort int,
	memberlistJoinAddrs []string,
	proxyAddresses map[string]string,
) *s2sproxy.Proxy {
	var muxConnectionType config.ConnectionType
	var muxAddressInfo config.TCPTLSInfo
	if muxMode == config.ServerMode {
		muxConnectionType = config.ConnTypeMuxServer
		muxAddressInfo = config.TCPTLSInfo{
			ConnectionString: muxAddress,
		}
	} else {
		muxConnectionType = config.ConnTypeMuxClient
		muxAddressInfo = config.TCPTLSInfo{
			ConnectionString: muxAddress,
		}
	}

	cfg := &config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{
			{
				Name: name,
				LocalServer: config.ClusterDefinition{
					Connection: config.TransportInfo{
						ConnectionType: config.ConnTypeTCP,
						TcpClient: config.TCPTLSInfo{
							ConnectionString: cluster.Host().FrontendGRPCAddress(),
						},
						TcpServer: config.TCPTLSInfo{
							ConnectionString: outboundAddress,
						},
					},
				},
				RemoteServer: config.ClusterDefinition{
					Connection: config.TransportInfo{
						ConnectionType: muxConnectionType,
						MuxCount:       1,
						MuxAddressInfo: muxAddressInfo,
					},
				},
				ShardCountConfig: shardCountConfig,
				MemberlistConfig: &config.MemberlistConfig{
					Enabled:        true,
					NodeName:       nodeName,
					BindAddr:       memberlistBindAddr,
					BindPort:       memberlistBindPort,
					JoinAddrs:      memberlistJoinAddrs,
					ProxyAddresses: proxyAddresses,
					TCPOnly:        true,
				},
			},
		},
	}

	configProvider := &simpleConfigProvider{cfg: *cfg}
	proxy := s2sproxy.NewProxy(configProvider, s.logger)
	s.NotNil(proxy)

	err := proxy.Start()
	s.NoError(err, "Failed to start proxy %s", name)

	s.logger.Info("Started proxy", tag.NewStringTag("name", name),
		tag.NewStringTag("inboundAddress", inboundAddress),
		tag.NewStringTag("outboundAddress", outboundAddress),
		tag.NewStringTag("muxAddress", muxAddress),
		tag.NewStringTag("muxMode", string(muxMode)),
		tag.NewStringTag("nodeName", nodeName),
	)

	return proxy
}

type trackingUpstreamServer struct {
	address string
	conns   atomic.Int64
	count1  *atomic.Int64
	count2  *atomic.Int64
}

type trackingUpstream struct {
	servers []*trackingUpstreamServer
	mu      sync.RWMutex
}

func (u *trackingUpstream) selectLeastConn() *trackingUpstreamServer {
	u.mu.RLock()
	defer u.mu.RUnlock()

	if len(u.servers) == 0 {
		return nil
	}

	selected := u.servers[0]
	minConns := selected.conns.Load()

	for i := 1; i < len(u.servers); i++ {
		conns := u.servers[i].conns.Load()
		if conns < minConns {
			minConns = conns
			selected = u.servers[i]
		}
	}

	if selected != nil {
		if selected == u.servers[0] {
			selected.count1.Add(1)
		} else if len(u.servers) > 1 && selected == u.servers[1] {
			selected.count2.Add(1)
		}
	}

	return selected
}

func (u *trackingUpstream) incrementConn(server *trackingUpstreamServer) {
	server.conns.Add(1)
}

func (u *trackingUpstream) decrementConn(server *trackingUpstreamServer) {
	server.conns.Add(-1)
}

type trackingTCPProxy struct {
	rules   []*trackingProxyRule
	logger  log.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	servers []net.Listener
}

type trackingProxyRule struct {
	ListenPort string
	Upstream   *trackingUpstream
}

func (p *trackingTCPProxy) Start() error {
	for _, rule := range p.rules {
		listener, err := net.Listen("tcp", ":"+rule.ListenPort)
		if err != nil {
			p.Stop()
			return fmt.Errorf("failed to listen on port %s: %w", rule.ListenPort, err)
		}
		p.servers = append(p.servers, listener)

		p.wg.Add(1)
		go p.handleListener(listener, rule)
	}

	return nil
}

func (p *trackingTCPProxy) Stop() {
	p.logger.Info("Stopping tracking TCP proxy")
	p.cancel()
	for _, server := range p.servers {
		p.logger.Info("Closing server", tag.NewStringTag("server", server.Addr().String()))
		_ = server.Close()
	}
	p.logger.Info("Waiting for goroutines to finish")
	p.wg.Wait()
	p.logger.Info("Tracking TCP proxy stopped")
}

func (p *trackingTCPProxy) handleListener(listener net.Listener, rule *trackingProxyRule) {
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		default:
		}

		clientConn, err := listener.Accept()
		if err != nil {
			select {
			case <-p.ctx.Done():
				return
			default:
				p.logger.Warn("failed to accept connection", tag.Error(err))
				continue
			}
		}

		p.wg.Add(1)
		go p.handleConnection(clientConn, rule)
	}
}

func (p *trackingTCPProxy) handleConnection(clientConn net.Conn, rule *trackingProxyRule) {
	defer p.wg.Done()
	defer func() { _ = clientConn.Close() }()

	// Check if already cancelled
	select {
	case <-p.ctx.Done():
		return
	default:
	}

	upstream := rule.Upstream.selectLeastConn()
	if upstream == nil {
		p.logger.Error("no upstream servers available")
		return
	}

	rule.Upstream.incrementConn(upstream)
	defer rule.Upstream.decrementConn(upstream)

	serverConn, err := net.DialTimeout("tcp", upstream.address, 5*time.Second)
	if err != nil {
		p.logger.Warn("failed to connect to upstream", tag.NewStringTag("upstream", upstream.address), tag.Error(err))
		return
	}
	defer func() { _ = serverConn.Close() }()

	// Close connections when context is cancelled to unblock io.Copy
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		<-p.ctx.Done()
		_ = clientConn.Close()
		_ = serverConn.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(serverConn, clientConn)
		_ = serverConn.Close()
	}()

	go func() {
		defer wg.Done()
		_, _ = io.Copy(clientConn, serverConn)
		_ = clientConn.Close()
	}()

	wg.Wait()
}

func (s *IntraProxyRoutingTestSuite) createLoadBalancer(
	listenPort string,
	upstreams []string,
	count1 *atomic.Int64,
	count2 *atomic.Int64,
) *trackingTCPProxy {
	trackingServers := make([]*trackingUpstreamServer, len(upstreams))
	for i, addr := range upstreams {
		trackingServers[i] = &trackingUpstreamServer{
			address: addr,
			count1:  count1,
			count2:  count2,
		}
	}

	trackingUpstream := &trackingUpstream{
		servers: trackingServers,
	}

	rules := []*trackingProxyRule{
		{
			ListenPort: listenPort,
			Upstream:   trackingUpstream,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	trackingProxy := &trackingTCPProxy{
		rules:  rules,
		logger: s.logger,
		ctx:    ctx,
		cancel: cancel,
	}

	err := trackingProxy.Start()
	s.NoError(err, "Failed to start load balancer on port %s", listenPort)

	return trackingProxy
}

func (s *IntraProxyRoutingTestSuite) configureRemoteCluster(
	cluster *testcore.TestCluster,
	remoteClusterName string,
	proxyAddress string,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := cluster.AdminClient().AddOrUpdateRemoteCluster(
		ctx,
		&adminservice.AddOrUpdateRemoteClusterRequest{
			FrontendAddress:               proxyAddress,
			EnableRemoteClusterConnection: true,
		},
	)
	s.NoError(err, "Failed to configure remote cluster %s", remoteClusterName)
	s.logger.Info("Configured remote cluster",
		tag.NewStringTag("remoteClusterName", remoteClusterName),
		tag.NewStringTag("proxyAddress", proxyAddress),
		tag.NewStringTag("clusterName", cluster.ClusterName()),
	)
}

func (s *IntraProxyRoutingTestSuite) removeRemoteCluster(
	cluster *testcore.TestCluster,
	remoteClusterName string,
) {
	_, err := cluster.AdminClient().RemoveRemoteCluster(
		context.Background(),
		&adminservice.RemoveRemoteClusterRequest{
			ClusterName: remoteClusterName,
		},
	)
	s.NoError(err, "Failed to remove remote cluster %s", remoteClusterName)
	s.logger.Info("Removed remote cluster",
		tag.NewStringTag("remoteClusterName", remoteClusterName),
		tag.NewStringTag("clusterName", cluster.ClusterName()),
	)
}

func (s *IntraProxyRoutingTestSuite) waitForReplicationReady() {
	time.Sleep(1 * time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, cluster := range []*testcore.TestCluster{s.clusterA, s.clusterB} {
		s.Eventually(func() bool {
			_, err := cluster.HistoryClient().GetReplicationStatus(
				ctx,
				&historyservice.GetReplicationStatusRequest{},
			)
			return err == nil
		}, 5*time.Second, 200*time.Millisecond, "Replication infrastructure not ready")
	}

	time.Sleep(1 * time.Second)
}

func (s *IntraProxyRoutingTestSuite) TestIntraProxyRoutingDistribution() {
	s.logger.Info("Testing intra-proxy routing distribution")

	ctx := context.Background()

	s.logger.Info("Triggering replication connections to verify distribution")

	var wg sync.WaitGroup
	numConnections := 20

	for i := 0; i < numConnections; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.clusterA.HistoryClient().GetReplicationStatus(
				ctx,
				&historyservice.GetReplicationStatusRequest{},
			)
			if err != nil {
				s.logger.Warn("GetReplicationStatus failed", tag.Error(err))
			}
		}()
	}

	wg.Wait()

	time.Sleep(2 * time.Second)

	countA1 := s.connectionCountsA1.Load()
	countA2 := s.connectionCountsA2.Load()
	countB1 := s.connectionCountsB1.Load()
	countB2 := s.connectionCountsB2.Load()
	countPA1 := s.connectionCountsPA1.Load()
	countPA2 := s.connectionCountsPA2.Load()

	s.logger.Info("Connection distribution results",
		tag.NewInt64("loadBalancerA_pa1", countA1),
		tag.NewInt64("loadBalancerA_pa2", countA2),
		tag.NewInt64("loadBalancerB_pb1_from_pa", countPA1),
		tag.NewInt64("loadBalancerB_pb2_from_pa", countPA2),
		tag.NewInt64("loadBalancerC_pb1", countB1),
		tag.NewInt64("loadBalancerC_pb2", countB2),
	)

	s.Greater(countA1, int64(0), "Load balancer A should route to pa1")
	s.Greater(countA2, int64(0), "Load balancer A should route to pa2")
	s.Greater(countB1, int64(0), "Load balancer C should route to pb1")
	s.Greater(countB2, int64(0), "Load balancer C should route to pb2")
	s.Greater(countPA1, int64(0), "Load balancer B should route to pb1 from pa")
	s.Greater(countPA2, int64(0), "Load balancer B should route to pb2 from pa")

	totalA := countA1 + countA2
	totalB := countB1 + countB2
	totalPA := countPA1 + countPA2

	s.logger.Info("Total connections",
		tag.NewInt64("totalA", totalA),
		tag.NewInt64("totalB", totalB),
		tag.NewInt64("totalPA", totalPA),
	)

	s.Greater(totalA, int64(0), "Should have connections through load balancer A")
	s.Greater(totalB, int64(0), "Should have connections through load balancer C")
	s.Greater(totalPA, int64(0), "Should have connections through load balancer B")
}
