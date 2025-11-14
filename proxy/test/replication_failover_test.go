package proxy

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	replicationpb "go.temporal.io/api/replication/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/cluster"
	"go.temporal.io/server/common/dynamicconfig"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.temporal.io/server/common/namespace"
	"go.temporal.io/server/tests/testcore"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/temporalio/s2s-proxy/config"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

type (
	// ReplicationTestSuite tests s2s-proxy replication and failover across multiple shard configurations
	ReplicationTestSuite struct {
		suite.Suite
		*require.Assertions

		logger log.Logger

		clusterA *testcore.TestCluster
		clusterB *testcore.TestCluster

		proxyA *s2sproxy.Proxy
		proxyB *s2sproxy.Proxy

		proxyAAddress string
		proxyBAddress string

		shardCountA      int
		shardCountB      int
		shardCountConfig config.ShardCountConfig
		namespace        string
		namespaceID      string
		startTime        time.Time

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
		Name             string
		ShardCountA      int
		ShardCountB      int
		WorkflowsPerPair int
		ShardCountConfig config.ShardCountConfig
	}
)

var testConfigs = []TestConfig{
	{
		Name:             "SingleShard",
		ShardCountA:      1,
		ShardCountB:      1,
		WorkflowsPerPair: 1,
	},
	{
		Name:             "FourShards",
		ShardCountA:      4,
		ShardCountB:      4,
		WorkflowsPerPair: 1,
	},
	{
		Name:             "AsymmetricShards_4to2",
		ShardCountA:      4,
		ShardCountB:      2,
		WorkflowsPerPair: 1,
	},
	{
		Name:             "AsymmetricShards_2to4",
		ShardCountA:      2,
		ShardCountB:      4,
		WorkflowsPerPair: 1,
	},
	{
		Name:             "ArbitraryShards_2to3_LCM",
		ShardCountA:      2,
		ShardCountB:      3,
		WorkflowsPerPair: 1,
		ShardCountConfig: config.ShardCountConfig{
			Mode:             config.ShardCountLCM,
			LocalShardCount:  2,
			RemoteShardCount: 3,
		},
	},
}

func TestReplicationFailoverTestSuite(t *testing.T) {
	for _, tc := range testConfigs {
		t.Run(tc.Name, func(t *testing.T) {
			s := &ReplicationTestSuite{
				shardCountA:      tc.ShardCountA,
				shardCountB:      tc.ShardCountB,
				shardCountConfig: tc.ShardCountConfig,
				workflowsPerPair: tc.WorkflowsPerPair,
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
	)

	s.clusterA = s.createCluster("cluster-a", s.shardCountA, 1)
	s.clusterB = s.createCluster("cluster-b", s.shardCountB, 2)

	basePort := 17000 + rand.Intn(10000)
	s.proxyAAddress = fmt.Sprintf("localhost:%d", basePort)
	proxyAOutbound := fmt.Sprintf("localhost:%d", basePort+1)
	s.proxyBAddress = fmt.Sprintf("localhost:%d", basePort+100)
	proxyBOutbound := fmt.Sprintf("localhost:%d", basePort+101)
	muxServerAddress := fmt.Sprintf("localhost:%d", basePort+200)

	proxyBShardConfig := s.shardCountConfig
	if proxyBShardConfig.Mode == config.ShardCountLCM {
		proxyBShardConfig.LocalShardCount = int32(s.shardCountB)
		proxyBShardConfig.RemoteShardCount = int32(s.shardCountA)
	}

	s.proxyA = s.createProxy("proxy-a", s.proxyAAddress, proxyAOutbound, muxServerAddress, s.clusterA, config.ClientMode, config.ShardCountConfig{})
	s.proxyB = s.createProxy("proxy-b", s.proxyBAddress, proxyBOutbound, muxServerAddress, s.clusterB, config.ServerMode, proxyBShardConfig)

	s.configureRemoteCluster(s.clusterA, s.clusterB.ClusterName(), proxyAOutbound)
	s.configureRemoteCluster(s.clusterB, s.clusterA.ClusterName(), proxyBOutbound)
	s.waitForReplicationReady()

	s.namespace = s.createGlobalNamespace()
	s.waitForClusterSynced()
}

func (s *ReplicationTestSuite) TearDownSuite() {
	if s.namespace != "" && s.clusterA != nil {
		s.deglobalizeNamespace(s.namespace)
	}

	if s.clusterA != nil && s.clusterB != nil {
		s.removeRemoteCluster(s.clusterA, s.clusterB.ClusterName())
		s.removeRemoteCluster(s.clusterB, s.clusterA.ClusterName())
	}
	if s.clusterA != nil {
		s.NoError(s.clusterA.TearDownCluster())
	}
	if s.clusterB != nil {
		s.NoError(s.clusterB.TearDownCluster())
	}
	if s.proxyA != nil {
		s.proxyA.Stop()
	}
	if s.proxyB != nil {
		s.proxyB.Stop()
	}

}

func (s *ReplicationTestSuite) SetupTest() {
	s.workflows = nil

	if s.namespace != "" {
		s.ensureNamespaceActive(s.clusterA.ClusterName())
	}
}

func (s *ReplicationTestSuite) createCluster(
	clusterName string,
	numShards int,
	initialFailoverVersion int64,
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
			NumHistoryHosts:  1,
		},
		DynamicConfigOverrides: map[dynamicconfig.Key]interface{}{
			dynamicconfig.NamespaceCacheRefreshInterval.Key(): time.Second,
			dynamicconfig.EnableReplicationStream.Key():       true,
			dynamicconfig.EnableReplicationTaskBatching.Key(): true,
		},
	}

	testClusterFactory := testcore.NewTestClusterFactory()
	cluster, err := testClusterFactory.NewCluster(s.T(), clusterConfig, s.logger)
	s.NoError(err, "Failed to create cluster %s", clusterName)
	s.NotNil(cluster)

	return cluster
}

func (s *ReplicationTestSuite) createProxy(
	name string,
	inboundAddress string,
	outboundAddress string,
	muxAddress string,
	cluster *testcore.TestCluster,
	muxMode config.MuxMode,
	shardCountConfig config.ShardCountConfig,
) *s2sproxy.Proxy {
	muxTransportName := "muxed"
	cfg := &config.S2SProxyConfig{
		Inbound: &config.ProxyConfig{
			Name: name + "-inbound",
			Server: config.ProxyServerConfig{
				Type:             config.MuxTransport,
				MuxTransportName: muxTransportName,
			},
			Client: config.ProxyClientConfig{
				Type: config.TCPTransport,
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: cluster.Host().FrontendGRPCAddress(),
				},
			},
		},
		Outbound: &config.ProxyConfig{
			Name: name + "-outbound",
			Server: config.ProxyServerConfig{
				Type: config.TCPTransport,
				TCPServerSetting: config.TCPServerSetting{
					ListenAddress: outboundAddress,
				},
			},
			Client: config.ProxyClientConfig{
				Type:             config.MuxTransport,
				MuxTransportName: muxTransportName,
			},
		},
		MuxTransports: []config.MuxTransportConfig{
			s.makeMuxTransportConfig(muxTransportName, muxMode, muxAddress, 1),
		},
		ShardCountConfig: shardCountConfig,
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
	)

	return proxy
}

func (s *ReplicationTestSuite) makeMuxTransportConfig(name string, mode config.MuxMode, address string, numConns int) config.MuxTransportConfig {
	if mode == config.ServerMode {
		return config.MuxTransportConfig{
			Name:           name,
			Mode:           mode,
			Client:         config.TCPClientSetting{},
			Server:         config.TCPServerSetting{ListenAddress: address},
			NumConnections: numConns,
		}
	} else {
		return config.MuxTransportConfig{
			Name:           name,
			Mode:           mode,
			Client:         config.TCPClientSetting{ServerAddress: address},
			Server:         config.TCPServerSetting{},
			NumConnections: numConns,
		}
	}
}

type simpleConfigProvider struct {
	cfg config.S2SProxyConfig
}

func (p *simpleConfigProvider) GetS2SProxyConfig() config.S2SProxyConfig {
	return p.cfg
}

func (s *ReplicationTestSuite) configureRemoteCluster(
	cluster *testcore.TestCluster,
	remoteClusterName string,
	proxyAddress string,
) {
	_, err := cluster.AdminClient().AddOrUpdateRemoteCluster(
		context.Background(),
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

func (s *ReplicationTestSuite) removeRemoteCluster(
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
func (s *ReplicationTestSuite) isValidShardPair(sourceShard int32, targetShard int32) bool {
	// If shard counts are equal, source and target shards must match
	// (same hash function with same shard count produces identical shard assignment)
	if s.shardCountA == s.shardCountB {
		return sourceShard == targetShard
	}

	// Convert to 0-based for modulo arithmetic
	sourceShard0 := sourceShard - 1
	targetShard0 := targetShard - 1

	// Case 1: targetShardCount divides sourceShardCount (e.g., 4 -> 2)
	// Source shard x maps to target shard (x % targetShardCount)
	if s.shardCountA%s.shardCountB == 0 {
		expectedTarget := sourceShard0 % int32(s.shardCountB)
		return targetShard0 == expectedTarget
	}

	// Case 2: sourceShardCount divides targetShardCount (e.g., 2 -> 4)
	// Source shard x can map to target shards in set {x, x+sourceShardCount, x+2*sourceShardCount, ...}
	// where all values are < targetShardCount
	if s.shardCountB%s.shardCountA == 0 {
		return targetShard0%int32(s.shardCountA) == sourceShard0
	}

	// No divisibility relationship, all pairs are possible (though may be hard to find)
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

func (s *ReplicationTestSuite) waitForReplicationReady() {
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
		resp, err := sourceCluster.HistoryClient().GetReplicationStatus(
			context.Background(),
			&historyservice.GetReplicationStatusRequest{},
		)
		if err != nil {
			s.logger.Debug("GetReplicationStatus failed", tag.Error(err))
			return false
		}

		if len(resp.Shards) == 0 {
			return false
		}

		for _, shard := range resp.Shards {
			if shard.MaxReplicationTaskId <= 0 {
				continue
			}

			remoteInfo, ok := shard.RemoteClusters[targetClusterName]
			if !ok || remoteInfo == nil {
				return false
			}

			if remoteInfo.AckedTaskId < shard.MaxReplicationTaskId {
				s.logger.Debug("Replication not synced",
					tag.ShardID(shard.ShardId),
					tag.NewInt64("maxTaskId", shard.MaxReplicationTaskId),
					tag.NewInt64("ackedTaskId", remoteInfo.AckedTaskId),
				)
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

	// TODO: make some progress on the workflows

	s.waitForClusterSynced()

	clientB := s.clusterB.FrontendClient()
	for _, wf := range s.workflows {
		s.NoError(s.verifyWorkflowReplicated(ctx, clientB, wf))
	}

	s.failoverNamespace(ctx, s.namespace, s.clusterB.ClusterName())

	for _, wf := range s.workflows {
		// TODO: continue the workflows instead of just terminating them
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
