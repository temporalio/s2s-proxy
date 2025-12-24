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
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	replicationpb "go.temporal.io/api/replication/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/tests/testcore"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

type SetupMode string

const (
	SetupModeSimple     SetupMode = "simple"     // Case B: Two proxies, direct connection, no load balancer, no memberlist
	SetupModeMultiProxy SetupMode = "multiproxy" // Case A: Multi-proxy with load balancers and memberlist
)

type (
	// ReplicationTestSuite tests s2s-proxy replication and failover across multiple shard configurations
	ReplicationTestSuite struct {
		suite.Suite
		*require.Assertions

		logger log.Logger

		clusterA *testcore.TestCluster
		clusterB *testcore.TestCluster

		// Case B: Simple setup
		proxyA *s2sproxy.Proxy
		proxyB *s2sproxy.Proxy

		proxyAOutbound string
		proxyBOutbound string

		// Case A: Multi-proxy setup
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

		setupMode SetupMode

		shardCountA       int
		shardCountB       int
		shardCountConfigB config.ShardCountConfig
		namespace         string
		namespaceID       string
		startTime         time.Time

		workflows []*WorkflowDistribution

		workflowsPerPair int
	}

	WorkflowDistribution struct {
		WorkflowID  string
		RunID       string
		SourceShard int32
		TargetShard int32
		TaskQueue   string
		Status      enumspb.WorkflowExecutionStatus
	}

	TestConfig struct {
		Name              string
		ShardCountA       int
		ShardCountB       int
		WorkflowsPerPair  int
		ShardCountConfigB config.ShardCountConfig
		SetupMode         SetupMode
	}
)

var testConfigs = []TestConfig{
	// Case B: Simple setup tests
	{
		Name:             "Simple_SingleShard",
		ShardCountA:      1,
		ShardCountB:      1,
		WorkflowsPerPair: 1,
		SetupMode:        SetupModeSimple,
	},
	{
		Name:             "Simple_FourShards",
		ShardCountA:      4,
		ShardCountB:      4,
		WorkflowsPerPair: 1,
		SetupMode:        SetupModeSimple,
	},
	{
		Name:             "Simple_AsymmetricShards_4to2",
		ShardCountA:      4,
		ShardCountB:      2,
		WorkflowsPerPair: 1,
		SetupMode:        SetupModeSimple,
	},
	{
		Name:             "Simple_AsymmetricShards_2to4",
		ShardCountA:      2,
		ShardCountB:      4,
		WorkflowsPerPair: 1,
		SetupMode:        SetupModeSimple,
	},
	{
		Name:             "Simple_ArbitraryShards_2to3_LCM",
		ShardCountA:      2,
		ShardCountB:      3,
		WorkflowsPerPair: 1,
		ShardCountConfigB: config.ShardCountConfig{
			Mode: config.ShardCountLCM,
		},
		SetupMode: SetupModeSimple,
	},
	{
		Name:             "Simple_ArbitraryShards_2to3_Routing",
		ShardCountA:      2,
		ShardCountB:      3,
		WorkflowsPerPair: 1,
		ShardCountConfigB: config.ShardCountConfig{
			Mode: config.ShardCountRouting,
		},
		SetupMode: SetupModeSimple,
	},
	// Case A: Multi-proxy setup tests
	{
		Name:             "MultiProxy_SingleShard",
		ShardCountA:      1,
		ShardCountB:      1,
		WorkflowsPerPair: 1,
		SetupMode:        SetupModeMultiProxy,
	},
	{
		Name:             "MultiProxy_FourShards",
		ShardCountA:      4,
		ShardCountB:      4,
		WorkflowsPerPair: 1,
		SetupMode:        SetupModeMultiProxy,
	},
	{
		Name:             "MultiProxy_AsymmetricShards_4to2",
		ShardCountA:      4,
		ShardCountB:      2,
		WorkflowsPerPair: 1,
		SetupMode:        SetupModeMultiProxy,
	},
	{
		Name:             "MultiProxy_AsymmetricShards_2to4",
		ShardCountA:      2,
		ShardCountB:      4,
		WorkflowsPerPair: 1,
		SetupMode:        SetupModeMultiProxy,
	},
	{
		Name:             "MultiProxy_ArbitraryShards_2to3_LCM",
		ShardCountA:      2,
		ShardCountB:      3,
		WorkflowsPerPair: 1,
		ShardCountConfigB: config.ShardCountConfig{
			Mode: config.ShardCountLCM,
		},
		SetupMode: SetupModeMultiProxy,
	},
	{
		Name:             "MultiProxy_ArbitraryShards_2to3_Routing",
		ShardCountA:      2,
		ShardCountB:      3,
		WorkflowsPerPair: 1,
		ShardCountConfigB: config.ShardCountConfig{
			Mode: config.ShardCountRouting,
		},
		SetupMode: SetupModeMultiProxy,
	},
}

func TestReplicationFailoverTestSuite(t *testing.T) {
	for _, tc := range testConfigs {
		t.Run(tc.Name, func(t *testing.T) {
			s := &ReplicationTestSuite{
				shardCountA:       tc.ShardCountA,
				shardCountB:       tc.ShardCountB,
				shardCountConfigB: tc.ShardCountConfigB,
				workflowsPerPair:  tc.WorkflowsPerPair,
				setupMode:         tc.SetupMode,
			}
			suite.Run(t, s)
		})
	}
}

func (s *ReplicationTestSuite) SetupSuite() {
	s.Assertions = require.New(s.T())
	s.logger = log.NewTestLogger()
	s.startTime = time.Now()

	s.logger.Info("Setting up replication test suite",
		tag.NewInt("shardCountA", s.shardCountA),
		tag.NewInt("shardCountB", s.shardCountB),
		tag.NewStringTag("setupMode", string(s.setupMode)),
	)

	s.clusterA = createCluster(s.logger, s.T(), "cluster-a", s.shardCountA, 1, 1)
	s.clusterB = createCluster(s.logger, s.T(), "cluster-b", s.shardCountB, 2, 1)

	if s.setupMode == SetupModeSimple {
		s.setupSimple()
	} else {
		s.setupMultiProxy()
	}

	s.logger.Info("Waiting for proxies to start and connect")
	time.Sleep(10 * time.Second)

	s.logger.Info("Configuring remote clusters")
	if s.setupMode == SetupModeSimple {
		configureRemoteCluster(s.logger, s.T(), s.clusterA, s.clusterB.ClusterName(), s.proxyAOutbound)
		configureRemoteCluster(s.logger, s.T(), s.clusterB, s.clusterA.ClusterName(), s.proxyBOutbound)
	} else {
		configureRemoteCluster(s.logger, s.T(), s.clusterA, s.clusterB.ClusterName(), fmt.Sprintf("localhost:%s", s.loadBalancerAPort))
		configureRemoteCluster(s.logger, s.T(), s.clusterB, s.clusterA.ClusterName(), fmt.Sprintf("localhost:%s", s.loadBalancerCPort))
	}

	waitForReplicationReady(s.logger, s.T(), s.clusterA, s.clusterB)

	s.namespace = s.createGlobalNamespace()
	s.waitForClusterSynced()
}

func (s *ReplicationTestSuite) setupSimple() {
	s.logger.Info("Setting up simple two-proxy configuration")

	proxyAOutbound := GetLocalhostAddress()
	proxyBOutbound := GetLocalhostAddress()
	muxServerAddress := GetLocalhostAddress()

	s.proxyAOutbound = proxyAOutbound
	s.proxyBOutbound = proxyBOutbound

	proxyBShardConfig := s.shardCountConfigB
	if proxyBShardConfig.Mode == config.ShardCountLCM || proxyBShardConfig.Mode == config.ShardCountRouting {
		proxyBShardConfig.LocalShardCount = int32(s.shardCountB)
		proxyBShardConfig.RemoteShardCount = int32(s.shardCountA)
	}

	s.proxyA = createProxy(s.logger, s.T(), "proxy-a", "", proxyAOutbound, muxServerAddress, s.clusterA, config.ClientMode, config.ShardCountConfig{}, "", "", 0, nil, nil)
	s.proxyB = createProxy(s.logger, s.T(), "proxy-b", "", proxyBOutbound, muxServerAddress, s.clusterB, config.ServerMode, proxyBShardConfig, "", "", 0, nil, nil)
}

func (s *ReplicationTestSuite) setupMultiProxy() {
	s.logger.Info("Setting up multi-proxy configuration with load balancers")

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

	// For intra-proxy communication, use outbound addresses where proxies listen
	proxyAddressesA := map[string]string{
		"proxy-node-a-1": s.proxyA1Outbound,
		"proxy-node-a-2": s.proxyA2Outbound,
	}
	proxyAddressesB := map[string]string{
		"proxy-node-b-1": s.proxyB1Outbound,
		"proxy-node-b-2": s.proxyB2Outbound,
	}

	s.proxyA1MemberlistPort = GetFreePort()
	s.proxyA2MemberlistPort = GetFreePort()
	s.proxyB1MemberlistPort = GetFreePort()
	s.proxyB2MemberlistPort = GetFreePort()

	proxyBShardConfig := s.shardCountConfigB
	if proxyBShardConfig.Mode == config.ShardCountLCM || proxyBShardConfig.Mode == config.ShardCountRouting {
		proxyBShardConfig.LocalShardCount = int32(s.shardCountB)
		proxyBShardConfig.RemoteShardCount = int32(s.shardCountA)
	}

	s.proxyB1 = createProxy(s.logger, s.T(), "proxy-b-1", proxyB1Address, s.proxyB1Outbound, s.proxyB1Mux, s.clusterB, config.ServerMode, proxyBShardConfig, "proxy-node-b-1", "127.0.0.1", s.proxyB1MemberlistPort, nil, proxyAddressesB)
	s.proxyB2 = createProxy(s.logger, s.T(), "proxy-b-2", proxyB2Address, s.proxyB2Outbound, s.proxyB2Mux, s.clusterB, config.ServerMode, proxyBShardConfig, "proxy-node-b-2", "127.0.0.1", s.proxyB2MemberlistPort, []string{fmt.Sprintf("127.0.0.1:%d", s.proxyB1MemberlistPort)}, proxyAddressesB)

	var countA1, countA2, countB1, countB2, countPA1, countPA2 atomic.Int64

	var err error
	s.loadBalancerA, err = createLoadBalancer(s.logger, loadBalancerAPort, []string{s.proxyA1Outbound, s.proxyA2Outbound}, &countA1, &countA2)
	s.NoError(err, "Failed to start load balancer A")
	s.loadBalancerB, err = createLoadBalancer(s.logger, loadBalancerBPort, []string{s.proxyB1Mux, s.proxyB2Mux}, &countPA1, &countPA2)
	s.NoError(err, "Failed to start load balancer B")
	s.loadBalancerC, err = createLoadBalancer(s.logger, loadBalancerCPort, []string{s.proxyB1Outbound, s.proxyB2Outbound}, &countB1, &countB2)
	s.NoError(err, "Failed to start load balancer C")

	muxLoadBalancerBAddress := fmt.Sprintf("localhost:%s", loadBalancerBPort)
	s.proxyA1 = createProxy(s.logger, s.T(), "proxy-a-1", proxyA1Address, s.proxyA1Outbound, muxLoadBalancerBAddress, s.clusterA, config.ClientMode, config.ShardCountConfig{}, "proxy-node-a-1", "127.0.0.1", s.proxyA1MemberlistPort, nil, proxyAddressesA)
	s.proxyA2 = createProxy(s.logger, s.T(), "proxy-a-2", proxyA2Address, s.proxyA2Outbound, muxLoadBalancerBAddress, s.clusterA, config.ClientMode, config.ShardCountConfig{}, "proxy-node-a-2", "127.0.0.1", s.proxyA2MemberlistPort, []string{fmt.Sprintf("127.0.0.1:%d", s.proxyA1MemberlistPort)}, proxyAddressesA)
}

func (s *ReplicationTestSuite) TearDownSuite() {
	if s.namespace != "" && s.clusterA != nil {
		s.deglobalizeNamespace(s.namespace)
	}

	if s.clusterA != nil && s.clusterB != nil {
		removeRemoteCluster(s.logger, s.T(), s.clusterA, s.clusterB.ClusterName())
		removeRemoteCluster(s.logger, s.T(), s.clusterB, s.clusterA.ClusterName())
	}
	if s.clusterA != nil {
		s.NoError(s.clusterA.TearDownCluster())
	}
	if s.clusterB != nil {
		s.NoError(s.clusterB.TearDownCluster())
	}

	if s.setupMode == SetupModeSimple {
		if s.proxyA != nil {
			s.proxyA.Stop()
		}
		if s.proxyB != nil {
			s.proxyB.Stop()
		}
	} else {
		if s.loadBalancerA != nil {
			s.loadBalancerA.Stop()
		}
		if s.loadBalancerB != nil {
			s.loadBalancerB.Stop()
		}
		if s.loadBalancerC != nil {
			s.loadBalancerC.Stop()
		}
		if s.proxyA1 != nil {
			s.proxyA1.Stop()
		}
		if s.proxyA2 != nil {
			s.proxyA2.Stop()
		}
		if s.proxyB1 != nil {
			s.proxyB1.Stop()
		}
		if s.proxyB2 != nil {
			s.proxyB2.Stop()
		}
	}
}

func (s *ReplicationTestSuite) SetupTest() {
	s.workflows = nil

	if s.namespace != "" {
		s.ensureNamespaceActive(s.clusterA.ClusterName())
	}
}

func (s *ReplicationTestSuite) createGlobalNamespace() string {
	ctx := context.Background()
	ns := fmt.Sprintf("test-ns-%s", common.GenerateRandomString(8))

	regReq := &workflowservice.RegisterNamespaceRequest{
		Namespace:         ns,
		IsGlobalNamespace: true,
		Clusters: []*replicationpb.ClusterReplicationConfig{
			{ClusterName: s.clusterA.ClusterName()},
			{ClusterName: s.clusterB.ClusterName()},
		},
		ActiveClusterName:                s.clusterA.ClusterName(),
		WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour),
	}

	_, err := s.clusterA.FrontendClient().RegisterNamespace(ctx, regReq)
	s.NoError(err, "Failed to register namespace")

	s.Eventually(func() bool {
		for _, c := range []*testcore.TestCluster{s.clusterA, s.clusterB} {
			for _, r := range c.Host().NamespaceRegistries() {
				resp, err := r.GetNamespace(namespace.Name(ns))
				if err != nil || resp == nil {
					return false
				}
				if !resp.IsGlobalNamespace() {
					return false
				}
			}
		}
		return true
	}, 10*time.Second, 200*time.Millisecond, "Namespace failed to replicate")

	descResp, err := s.clusterA.FrontendClient().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
		Namespace: ns,
	})
	s.NoError(err)
	s.namespaceID = descResp.NamespaceInfo.GetId()

	return ns
}

func (s *ReplicationTestSuite) generateWorkflowsWithLoad(workflowsPerPair int) []*WorkflowDistribution {
	var workflows []*WorkflowDistribution
	expectedValidPairs := 0

	for sourceShard := int32(1); sourceShard <= int32(s.shardCountA); sourceShard++ {
		for targetShard := int32(1); targetShard <= int32(s.shardCountB); targetShard++ {
			if !s.isValidShardPair(sourceShard, targetShard) {
				s.logger.Debug("Skipping invalid shard pair",
					tag.NewInt32("sourceShard", sourceShard),
					tag.NewInt32("targetShard", targetShard),
					tag.NewInt("sourceShardCount", s.shardCountA),
					tag.NewInt("targetShardCount", s.shardCountB),
				)
				continue
			}
			expectedValidPairs++

			for i := 0; i < workflowsPerPair; i++ {
				workflowID := s.findWorkflowIDForShardPairWithIndex(sourceShard, targetShard, i)

				actualSourceShard := s.calculateTargetShard(workflowID, s.shardCountA)
				actualTargetShard := s.calculateTargetShard(workflowID, s.shardCountB)
				if actualSourceShard != sourceShard || actualTargetShard != targetShard {
					s.Fail(fmt.Sprintf("WorkflowID %s does not map to expected shard pair: expected (%d, %d), got (%d, %d)",
						workflowID, sourceShard, targetShard, actualSourceShard, actualTargetShard))
					continue
				}

				workflows = append(workflows, &WorkflowDistribution{
					WorkflowID:  workflowID,
					SourceShard: sourceShard,
					TargetShard: targetShard,
					TaskQueue:   fmt.Sprintf("tq-s%d-t%d-%d", sourceShard, targetShard, i),
					Status:      enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING,
				})
			}
		}
	}

	expectedWorkflows := expectedValidPairs * s.workflowsPerPair
	s.Len(workflows, expectedWorkflows, "Should have correct number of workflows (accounting for invalid pairs)")

	s.logger.Info("Generated workflows",
		tag.NewInt("expectedValidPairs", expectedValidPairs),
		tag.NewInt("generatedWorkflows", len(workflows)),
	)

	return workflows
}

// isValidShardPair checks if a (sourceShard, targetShard) pair is mathematically possible
// When shard counts have a divisibility relationship, certain pairs are impossible.
// Shards are 1-based in the test but converted to 0-based for modulo arithmetic.
// For example, if sourceShardCount=4 and targetShardCount=4 (equal):
//   - sourceShard must equal targetShard (same hash function with same shard count produces same result)
//   - Only pairs (1,1), (2,2), (3,3), (4,4) are valid
//
// For example, if sourceShardCount=4 and targetShardCount=2:
//   - sourceShard 1 (0-based: 0) can only map to targetShard 1 (0-based: 0)
//   - sourceShard 2 (0-based: 1) can only map to targetShard 2 (0-based: 1)
//   - sourceShard 3 (0-based: 2) can only map to targetShard 1 (0-based: 0)
//   - sourceShard 4 (0-based: 3) can only map to targetShard 2 (0-based: 1)
//
// Vice versa: if sourceShardCount=2 and targetShardCount=4:
//   - sourceShard 1 (0-based: 0) can only map to targetShard 1 or 3 (0-based: 0 or 2)
//   - sourceShard 2 (0-based: 1) can only map to targetShard 2 or 4 (0-based: 1 or 3)
//
// When using routing mode, all pairs are valid because intra-proxy routing can handle arbitrary mappings.
func (s *ReplicationTestSuite) isValidShardPair(sourceShard int32, targetShard int32) bool {
	// In routing mode, all pairs are valid because intra-proxy routing can handle arbitrary mappings
	if s.shardCountConfigB.Mode == config.ShardCountRouting {
		return true
	}

	if s.shardCountA == s.shardCountB {
		return sourceShard == targetShard
	}

	sourceShard0 := sourceShard - 1
	targetShard0 := targetShard - 1

	if s.shardCountA%s.shardCountB == 0 {
		expectedTarget := sourceShard0 % int32(s.shardCountB)
		return targetShard0 == expectedTarget
	}

	if s.shardCountB%s.shardCountA == 0 {
		return targetShard0%int32(s.shardCountA) == sourceShard0
	}

	return true
}

func (s *ReplicationTestSuite) calculateTargetShard(workflowID string, numShards int) int32 {
	return common.WorkflowIDToHistoryShard(
		s.namespaceID,
		workflowID,
		int32(numShards),
	)
}

func (s *ReplicationTestSuite) findWorkflowIDForShardPairWithIndex(sourceShard int32, targetShard int32, index int) string {
	if s.namespaceID == "" {
		s.Fail("namespaceID is not set - cannot calculate shard assignments")
		return ""
	}

	for i := 0; i < 100000; i++ {
		candidate := fmt.Sprintf("wf-s%d-t%d-%d-%d", sourceShard, targetShard, index, i)
		actualSourceShard := s.calculateTargetShard(candidate, s.shardCountA)
		actualTargetShard := s.calculateTargetShard(candidate, s.shardCountB)

		if actualSourceShard == sourceShard && actualTargetShard == targetShard {
			return candidate
		}
	}
	s.Fail(fmt.Sprintf("Could not find workflow ID for shard pair (%d, %d) with index %d and namespaceID %s after 100000 attempts", sourceShard, targetShard, index, s.namespaceID))
	return ""
}

func (s *ReplicationTestSuite) waitForClusterSynced() {
	s.waitForClusterConnected(s.clusterA, s.clusterB.ClusterName())
	s.waitForClusterConnected(s.clusterB, s.clusterA.ClusterName())
}

func (s *ReplicationTestSuite) waitForClusterConnected(
	sourceCluster *testcore.TestCluster,
	targetClusterName string,
) {
	s.logger.Info("Waiting for clusters to sync",
		tag.NewStringTag("source", sourceCluster.ClusterName()),
		tag.NewStringTag("target", targetClusterName),
	)

	s.Eventually(func() bool {
		s.logger.Info("Checking replication status for clusters to sync",
			tag.NewStringTag("source", sourceCluster.ClusterName()),
			tag.NewStringTag("target", targetClusterName),
		)

		resp, err := sourceCluster.HistoryClient().GetReplicationStatus(
			context.Background(),
			&historyservice.GetReplicationStatusRequest{},
		)
		if err != nil {
			s.logger.Debug("GetReplicationStatus failed", tag.Error(err))
			return false
		}
		s.logger.Info("GetReplicationStatus succeeded",
			tag.NewStringTag("source", sourceCluster.ClusterName()),
			tag.NewStringTag("target", targetClusterName),
			tag.NewStringTag("resp", fmt.Sprintf("%v", resp)),
		)

		if len(resp.Shards) == 0 {
			return false
		}

		for _, shard := range resp.Shards {
			s.logger.Info("Checking shard",
				tag.NewInt32("shardId", shard.ShardId),
				tag.NewInt64("maxReplicationTaskId", shard.MaxReplicationTaskId),
				tag.NewStringTag("shardLocalTime", fmt.Sprintf("%v", shard.ShardLocalTime.AsTime())),
				tag.NewStringTag("remoteClusters", fmt.Sprintf("%v", shard.RemoteClusters)),
			)
			if shard.MaxReplicationTaskId <= 0 {
				continue
			}

			s.NotNil(shard.ShardLocalTime)
			s.WithinRange(shard.ShardLocalTime.AsTime(), s.startTime, time.Now())

			remoteInfo, ok := shard.RemoteClusters[targetClusterName]
			if !ok || remoteInfo == nil {
				return false
			}

			if remoteInfo.AckedTaskId < shard.MaxReplicationTaskId {
				return false
			}
		}
		return true
	}, 30*time.Second, 500*time.Millisecond, "Clusters failed to sync")

	s.logger.Info("Clusters synced",
		tag.NewStringTag("source", sourceCluster.ClusterName()),
		tag.NewStringTag("target", targetClusterName),
	)
}

func (s *ReplicationTestSuite) TestReplication() {
	ctx := context.Background()

	s.workflows = s.generateWorkflowsWithLoad(s.workflowsPerPair)

	s.verifyShardDistribution(s.workflows)

	clientA := s.clusterA.FrontendClient()
	useConcurrent := s.workflowsPerPair > 1

	if useConcurrent {
		var wg sync.WaitGroup
		errors := make(chan error, len(s.workflows))

		for _, wf := range s.workflows {
			wg.Add(1)
			go func(w *WorkflowDistribution) {
				defer wg.Done()
				if errStart := s.startSimpleWorkflow(ctx, clientA, w); errStart != nil {
					errors <- errStart
				}
			}(wf)
		}

		wg.Wait()
		close(errors)

		errorCount := 0
		for errStart := range errors {
			s.logger.Error("Failed to start workflow", tag.Error(errStart))
			errorCount++
		}
		s.Equal(0, errorCount, "Some workflows failed to start")
	} else {
		for _, wf := range s.workflows {
			s.NoError(s.startSimpleWorkflow(ctx, clientA, wf))
		}
	}

	s.waitForClusterSynced()

	clientB := s.clusterB.FrontendClient()
	for _, wf := range s.workflows {
		s.NoError(s.verifyWorkflowReplicated(ctx, clientB, wf))
	}

	s.failoverNamespace(ctx, s.namespace, s.clusterB.ClusterName())

	for _, wf := range s.workflows {
		s.completeWorkflow(ctx, clientB, wf)
	}

	s.waitForClusterSynced()

	for _, wf := range s.workflows {
		descA, err := clientA.DescribeWorkflowExecution(ctx, &workflowservice.DescribeWorkflowExecutionRequest{
			Namespace: s.namespace,
			Execution: &commonpb.WorkflowExecution{
				WorkflowId: wf.WorkflowID,
			},
		})
		s.NoError(err, "Failed to describe workflow on Cluster A")
		s.Equal(enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED, descA.WorkflowExecutionInfo.Status)

		descB, err := clientB.DescribeWorkflowExecution(ctx, &workflowservice.DescribeWorkflowExecutionRequest{
			Namespace: s.namespace,
			Execution: &commonpb.WorkflowExecution{
				WorkflowId: wf.WorkflowID,
			},
		})
		s.NoError(err, "Failed to describe workflow on Cluster B")
		s.Equal(enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED, descB.WorkflowExecutionInfo.Status)
	}

	s.logger.Info("Failover completed successfully with workflow continuity")
}

func (s *ReplicationTestSuite) startSimpleWorkflow(
	ctx context.Context,
	client workflowservice.WorkflowServiceClient,
	wf *WorkflowDistribution,
) error {
	startReq := &workflowservice.StartWorkflowExecutionRequest{
		RequestId:           common.GenerateRandomString(16),
		Namespace:           s.namespace,
		WorkflowId:          wf.WorkflowID,
		WorkflowType:        &commonpb.WorkflowType{Name: "test-workflow"},
		TaskQueue:           &taskqueuepb.TaskQueue{Name: wf.TaskQueue, Kind: enumspb.TASK_QUEUE_KIND_NORMAL},
		Input:               nil,
		WorkflowRunTimeout:  durationpb.New(300 * time.Second),
		WorkflowTaskTimeout: durationpb.New(10 * time.Second),
		Identity:            "test-worker",
	}

	resp, err := client.StartWorkflowExecution(ctx, startReq)
	if err != nil {
		return err
	}

	wf.RunID = resp.GetRunId()
	wf.Status = enumspb.WORKFLOW_EXECUTION_STATUS_RUNNING
	return nil
}

func (s *ReplicationTestSuite) completeWorkflow(
	ctx context.Context,
	client workflowservice.WorkflowServiceClient,
	wf *WorkflowDistribution,
) {
	_, err := client.TerminateWorkflowExecution(ctx, &workflowservice.TerminateWorkflowExecutionRequest{
		Namespace: s.namespace,
		WorkflowExecution: &commonpb.WorkflowExecution{
			WorkflowId: wf.WorkflowID,
		},
		Reason:   "test-completion",
		Identity: "test-worker",
	})
	s.NoError(err, "Failed to complete workflow %s", wf.WorkflowID)
	wf.Status = enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED
}

func (s *ReplicationTestSuite) verifyWorkflowReplicated(
	ctx context.Context,
	client workflowservice.WorkflowServiceClient,
	wf *WorkflowDistribution,
) error {
	s.Eventually(func() bool {
		_, err := client.DescribeWorkflowExecution(ctx, &workflowservice.DescribeWorkflowExecutionRequest{
			Namespace: s.namespace,
			Execution: &commonpb.WorkflowExecution{
				WorkflowId: wf.WorkflowID,
			},
		})
		return err == nil
	}, 10*time.Second, 200*time.Millisecond, "Workflow %s not replicated", wf.WorkflowID)

	return nil
}

func (s *ReplicationTestSuite) failoverNamespace(
	ctx context.Context,
	namespaceName string,
	targetCluster string,
) {
	s.logger.Info("Failing over namespace", tag.NewStringTag("namespace", namespaceName), tag.NewStringTag("targetCluster", targetCluster))

	s.waitForClusterSynced()

	updateReq := &workflowservice.UpdateNamespaceRequest{
		Namespace: namespaceName,
		ReplicationConfig: &replicationpb.NamespaceReplicationConfig{
			ActiveClusterName: targetCluster,
		},
	}

	_, err := s.clusterA.FrontendClient().UpdateNamespace(ctx, updateReq)
	s.NoError(err, "Failover failed")

	s.Eventually(func() bool {
		for _, c := range []*testcore.TestCluster{s.clusterA, s.clusterB} {
			for _, r := range c.Host().NamespaceRegistries() {
				resp, err := r.GetNamespace(namespace.Name(namespaceName))
				if err != nil || resp == nil {
					return false
				}
				if resp.ActiveClusterName() != targetCluster {
					return false
				}
			}
		}
		return true
	}, 10*time.Second, 200*time.Millisecond, "Namespace failover not propagated")

	s.waitForClusterSynced()

	s.logger.Info("Namespace failover completed", tag.NewStringTag("namespace", namespaceName), tag.NewStringTag("targetCluster", targetCluster))
}

func (s *ReplicationTestSuite) deglobalizeNamespace(namespaceName string) {
	if s.clusterA == nil {
		return
	}

	ctx := context.Background()
	updateReq := &workflowservice.UpdateNamespaceRequest{
		Namespace: namespaceName,
		ReplicationConfig: &replicationpb.NamespaceReplicationConfig{
			ActiveClusterName: s.clusterA.ClusterName(),
			Clusters: []*replicationpb.ClusterReplicationConfig{
				{ClusterName: s.clusterA.ClusterName()},
			},
		},
	}

	_, err := s.clusterA.FrontendClient().UpdateNamespace(ctx, updateReq)
	if err != nil {
		s.logger.Warn("Failed to deglobalize namespace", tag.NewStringTag("namespace", namespaceName), tag.Error(err))
		return
	}

	s.Eventually(func() bool {
		for _, c := range []*testcore.TestCluster{s.clusterA, s.clusterB} {
			if c == nil {
				continue
			}
			descResp, err := c.FrontendClient().DescribeNamespace(ctx, &workflowservice.DescribeNamespaceRequest{
				Namespace: namespaceName,
			})
			if err != nil || descResp == nil {
				return false
			}
			clusters := descResp.ReplicationConfig.GetClusters()
			if len(clusters) != 1 {
				return false
			}
			if clusters[0].GetClusterName() != s.clusterA.ClusterName() {
				return false
			}
		}
		return true
	}, 10*time.Second, 200*time.Millisecond, "Namespace deglobalization not propagated")

	s.logger.Info("Deglobalized namespace", tag.NewStringTag("namespace", namespaceName))
}

func (s *ReplicationTestSuite) ensureNamespaceActive(targetCluster string) {
	descResp, err := s.clusterA.FrontendClient().DescribeNamespace(context.Background(), &workflowservice.DescribeNamespaceRequest{
		Namespace: s.namespace,
	})
	if err != nil {
		s.logger.Warn("Failed to describe namespace", tag.Error(err))
		return
	}

	currentActive := descResp.ReplicationConfig.GetActiveClusterName()
	if currentActive == targetCluster {
		return
	}

	s.logger.Info("Resetting namespace to active cluster",
		tag.NewStringTag("namespace", s.namespace),
		tag.NewStringTag("from", currentActive),
		tag.NewStringTag("to", targetCluster),
	)

	s.failoverNamespace(context.Background(), s.namespace, targetCluster)
}

func (s *ReplicationTestSuite) verifyShardDistribution(workflows []*WorkflowDistribution) {
	sourceShardCoverage := make(map[int32]int)
	targetShardCoverage := make(map[int32]int)

	for _, wf := range workflows {
		actualSourceShard := s.calculateTargetShard(wf.WorkflowID, s.shardCountA)
		s.Equal(wf.SourceShard, actualSourceShard, "Workflow %s does not map to expected source shard", wf.WorkflowID)

		actualTargetShard := s.calculateTargetShard(wf.WorkflowID, s.shardCountB)
		s.Equal(wf.TargetShard, actualTargetShard, "Workflow %s does not map to expected target shard", wf.WorkflowID)

		sourceShardCoverage[wf.SourceShard]++
		targetShardCoverage[wf.TargetShard]++
	}

	for shard := int32(1); shard <= int32(s.shardCountA); shard++ {
		count := sourceShardCoverage[shard]
		s.Greater(count, 0, "No workflows on source shard %d", shard)
	}

	for shard := int32(1); shard <= int32(s.shardCountB); shard++ {
		count := targetShardCoverage[shard]
		s.Greater(count, 0, "No workflows on target shard %d", shard)
	}

	s.logger.Info("Shard distribution verified",
		tag.NewStringTag("sourceCoverage", fmt.Sprintf("%v", sourceShardCoverage)),
		tag.NewStringTag("targetCoverage", fmt.Sprintf("%v", targetShardCoverage)),
	)
}
