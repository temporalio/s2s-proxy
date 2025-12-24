package proxy

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/tests/testcore"

	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
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

	s.clusterA = createCluster(s.logger, s.T(), "cluster-a", 2, 1, 1)
	s.clusterB = createCluster(s.logger, s.T(), "cluster-b", 2, 2, 1)

	s.proxyA1Outbound = GetLocalhostAddress()
	s.proxyA2Outbound = GetLocalhostAddress()
	s.proxyB1Outbound = GetLocalhostAddress()
	s.proxyB2Outbound = GetLocalhostAddress()

	s.proxyB1Mux = GetLocalhostAddress()
	s.proxyB2Mux = GetLocalhostAddress()

	loadBalancerAPort := fmt.Sprintf("%d", GetFreePort())
	loadBalancerBPort := fmt.Sprintf("%d", GetFreePort())
	loadBalancerCPort := fmt.Sprintf("%d", GetFreePort())

	s.loadBalancerAPort = loadBalancerAPort
	s.loadBalancerBPort = loadBalancerBPort
	s.loadBalancerCPort = loadBalancerCPort

	proxyA1Address := GetLocalhostAddress()
	proxyA2Address := GetLocalhostAddress()
	proxyB1Address := GetLocalhostAddress()
	proxyB2Address := GetLocalhostAddress()

	proxyAddressesA := map[string]string{
		"proxy-node-a-1": proxyA1Address,
		"proxy-node-a-2": proxyA2Address,
	}
	proxyAddressesB := map[string]string{
		"proxy-node-b-1": proxyB1Address,
		"proxy-node-b-2": proxyB2Address,
	}

	s.proxyA1MemberlistPort = GetFreePort()
	s.proxyA2MemberlistPort = GetFreePort()
	s.proxyB1MemberlistPort = GetFreePort()
	s.proxyB2MemberlistPort = GetFreePort()

	s.proxyB1 = createProxy(s.logger, s.T(), "proxy-b-1", proxyB1Address, s.proxyB1Outbound, s.proxyB1Mux, s.clusterB, config.ServerMode, config.ShardCountConfig{}, "proxy-node-b-1", "127.0.0.1", s.proxyB1MemberlistPort, nil, proxyAddressesB)
	s.proxyB2 = createProxy(s.logger, s.T(), "proxy-b-2", proxyB2Address, s.proxyB2Outbound, s.proxyB2Mux, s.clusterB, config.ServerMode, config.ShardCountConfig{}, "proxy-node-b-2", "127.0.0.1", s.proxyB2MemberlistPort, []string{fmt.Sprintf("127.0.0.1:%d", s.proxyB1MemberlistPort)}, proxyAddressesB)

	s.logger.Info("Setting up load balancers")

	var err error
	s.loadBalancerA, err = createLoadBalancer(s.logger, loadBalancerAPort, []string{s.proxyA1Outbound, s.proxyA2Outbound}, &s.connectionCountsA1, &s.connectionCountsA2)
	s.NoError(err, "Failed to start load balancer A")
	s.loadBalancerB, err = createLoadBalancer(s.logger, loadBalancerBPort, []string{s.proxyB1Mux, s.proxyB2Mux}, &s.connectionCountsPA1, &s.connectionCountsPA2)
	s.NoError(err, "Failed to start load balancer B")
	s.loadBalancerC, err = createLoadBalancer(s.logger, loadBalancerCPort, []string{s.proxyB1Outbound, s.proxyB2Outbound}, &s.connectionCountsB1, &s.connectionCountsB2)
	s.NoError(err, "Failed to start load balancer C")

	muxLoadBalancerBAddress := fmt.Sprintf("localhost:%s", loadBalancerBPort)
	s.proxyA1 = createProxy(s.logger, s.T(), "proxy-a-1", proxyA1Address, s.proxyA1Outbound, muxLoadBalancerBAddress, s.clusterA, config.ClientMode, config.ShardCountConfig{}, "proxy-node-a-1", "127.0.0.1", s.proxyA1MemberlistPort, nil, proxyAddressesA)
	s.proxyA2 = createProxy(s.logger, s.T(), "proxy-a-2", proxyA2Address, s.proxyA2Outbound, muxLoadBalancerBAddress, s.clusterA, config.ClientMode, config.ShardCountConfig{}, "proxy-node-a-2", "127.0.0.1", s.proxyA2MemberlistPort, []string{fmt.Sprintf("127.0.0.1:%d", s.proxyA1MemberlistPort)}, proxyAddressesA)

	s.logger.Info("Waiting for proxies to start and connect")
	time.Sleep(15 * time.Second)

	s.logger.Info("Configuring remote clusters")
	configureRemoteCluster(s.logger, s.T(), s.clusterA, s.clusterB.ClusterName(), fmt.Sprintf("localhost:%s", loadBalancerAPort))
	configureRemoteCluster(s.logger, s.T(), s.clusterB, s.clusterA.ClusterName(), fmt.Sprintf("localhost:%s", loadBalancerCPort))
	waitForReplicationReady(s.logger, s.T(), s.clusterA, s.clusterB)
}

func (s *IntraProxyRoutingTestSuite) TearDownSuite() {
	s.logger.Info("Tearing down intra-proxy routing test suite")
	if s.clusterA != nil && s.clusterB != nil {
		s.logger.Info("Removing remote cluster A from cluster B")
		removeRemoteCluster(s.logger, s.T(), s.clusterA, s.clusterB.ClusterName())
		s.logger.Info("Remote cluster A removed")
		s.logger.Info("Removing remote cluster B from cluster A")
		removeRemoteCluster(s.logger, s.T(), s.clusterB, s.clusterA.ClusterName())
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
