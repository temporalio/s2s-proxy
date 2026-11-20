package proxy

import (
	"context"
	"math"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	namespacepb "go.temporal.io/api/namespace/v1"
	replicationpb "go.temporal.io/api/replication/v1"
	adminservicepb "go.temporal.io/server/api/adminservice/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.uber.org/mock/gomock"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
)

func proxyRewriteConfig(mode config.NamespaceFailoverVersionRewriteMode) *config.NamespaceFailoverVersionRewriteConfig {
	return &config.NamespaceFailoverVersionRewriteConfig{
		Mode:       mode,
		Namespaces: []string{"migration"},
		Local: config.NamespaceFailoverVersionEndpoint{
			ClusterName: "self-hosted", InitialFailoverVersion: 1,
		},
		Remote: config.NamespaceFailoverVersionEndpoint{
			ClusterName: "cloud", InitialFailoverVersion: 2,
		},
	}
}

func testNamespaceRewriters(
	t *testing.T,
	mode config.NamespaceFailoverVersionRewriteMode,
	translation config.StringTranslator,
) (*NamespaceFailoverVersionRewriter, *NamespaceFailoverVersionRewriter) {
	t.Helper()
	namespaces, err := translation.AsLocalToRemoteBiMap()
	require.NoError(t, err)
	localToRemote, remoteToLocal, err := newNamespaceFailoverVersionRewriters(
		"test", proxyRewriteConfig(mode), config.IntMapping{Local: 10, Remote: 7}, namespaces,
	)
	require.NoError(t, err)
	return localToRemote, remoteToLocal
}

func TestNamespaceFailoverVersionRewriteBothDirections(t *testing.T) {
	localToRemote, remoteToLocal := testNamespaceRewriters(t, config.NamespaceFailoverVersionRewriteModeEnforce, config.StringTranslator{})

	forward := proxyNamespaceResponse("migration", "cloud", 9, 9)
	observations, err := localToRemote.rewriteGetNamespace(forward)
	require.NoError(t, err)
	require.EqualValues(t, 12, forward.GetFailoverVersion())
	require.EqualValues(t, 12, forward.GetFailoverHistory()[1].GetFailoverVersion())
	require.Equal(t, namespaceRewriteResultRewritten, observations[0].result)
	task := proxyNamespaceTask("migration", "cloud", 9, 9)
	messages := &adminservicepb.GetNamespaceReplicationMessagesResponse{
		Messages: &replicationspb.ReplicationMessages{ReplicationTasks: []*replicationspb.ReplicationTask{task}},
	}
	observations, err = localToRemote.rewriteNamespaceReplicationMessages(messages)
	require.NoError(t, err)
	require.EqualValues(t, forward.GetFailoverVersion(), task.GetNamespaceTaskAttributes().GetFailoverVersion())
	require.Equal(t, namespaceRewriteOperationTask, observations[0].operation)

	reverse := proxyNamespaceResponse("migration", "self-hosted", 21, 21)
	observations, err = remoteToLocal.rewriteGetNamespace(reverse)
	require.NoError(t, err)
	require.EqualValues(t, 29, reverse.GetFailoverVersion(), "22 must be skipped because Cloud sees it as lane 2")
	require.Equal(t, namespaceRewriteResultRewritten, observations[0].result)

	retry := proxyNamespaceResponse("migration", "cloud", 12, 12)
	observations, err = localToRemote.rewriteGetNamespace(retry)
	require.NoError(t, err)
	require.EqualValues(t, 12, retry.GetFailoverVersion())
	require.Equal(t, namespaceRewriteResultNotCutover, observations[0].result)
}

func TestNamespaceFailoverVersionObserveAndScope(t *testing.T) {
	localToRemote, _ := testNamespaceRewriters(t, config.NamespaceFailoverVersionRewriteModeObserve, config.StringTranslator{})

	wouldRewrite := proxyNamespaceResponse("migration", "cloud", 9, 9)
	observations, err := localToRemote.rewriteGetNamespace(wouldRewrite)
	require.NoError(t, err)
	require.EqualValues(t, 9, wouldRewrite.GetFailoverVersion())
	require.Equal(t, namespaceRewriteResultWouldRewrite, observations[0].result)
	require.EqualValues(t, 12, observations[0].output)

	outOfScope := proxyNamespaceResponse("other", "cloud", 9, 9)
	observations, err = localToRemote.rewriteGetNamespace(outOfScope)
	require.NoError(t, err)
	require.Equal(t, namespaceRewriteResultOutOfScope, observations[0].result)

	malformed := proxyNamespaceResponse("migration", "cloud", 9, 8)
	observations, err = localToRemote.rewriteGetNamespace(malformed)
	require.NoError(t, err, "observe reports malformed candidates without failing the RPC")
	require.Equal(t, namespaceRewriteResultError, observations[0].result)
	require.Equal(t, "history_mismatch", observations[0].reason)
}

func TestNamespaceFailoverVersionBatchIsAtomic(t *testing.T) {
	localToRemote, _ := testNamespaceRewriters(t, config.NamespaceFailoverVersionRewriteModeEnforce, config.StringTranslator{})
	valid := proxyNamespaceTask("migration", "cloud", 9, 9)
	invalid := proxyNamespaceTask("migration", "cloud", 9, 8)
	response := &adminservicepb.GetNamespaceReplicationMessagesResponse{
		Messages: &replicationspb.ReplicationMessages{
			ReplicationTasks: []*replicationspb.ReplicationTask{valid, invalid},
		},
	}

	_, err := localToRemote.rewriteNamespaceReplicationMessages(response)
	require.ErrorContains(t, err, "history_mismatch")
	require.EqualValues(t, 9, valid.GetNamespaceTaskAttributes().GetFailoverVersion())
	require.EqualValues(t, 9, invalid.GetNamespaceTaskAttributes().GetFailoverVersion())
}

func TestNamespaceFailoverVersionRejectsThirdCluster(t *testing.T) {
	localToRemote, _ := testNamespaceRewriters(t, config.NamespaceFailoverVersionRewriteModeEnforce, config.StringTranslator{})
	response := proxyNamespaceResponse("migration", "self-hosted", 8, 8)
	response.ReplicationConfig.Clusters = append(
		response.ReplicationConfig.Clusters,
		&replicationpb.ClusterReplicationConfig{ClusterName: "third"},
	)

	observations, err := localToRemote.rewriteGetNamespace(response)
	require.ErrorContains(t, err, "unsupported_topology")
	require.Equal(t, namespaceRewriteResultError, observations[0].result)
}

func TestNamespaceFailoverVersionUsesRemoteNamespaceName(t *testing.T) {
	translation := config.StringTranslator{Mappings: []config.StringMapping{{Local: "migration", Remote: "cloud-migration"}}}
	_, remoteToLocal := testNamespaceRewriters(t, config.NamespaceFailoverVersionRewriteModeEnforce, translation)
	response := proxyNamespaceResponse("cloud-migration", "self-hosted", 21, 21)

	observations, err := remoteToLocal.rewriteGetNamespace(response)
	require.NoError(t, err)
	require.EqualValues(t, 29, response.GetFailoverVersion())
	require.Equal(t, "migration", observations[0].namespace, "logs use the canonical local namespace")
}

func TestNamespaceFailoverVersionArithmeticAgainstReference(t *testing.T) {
	for senderFVI := int64(3); senderFVI <= 20; senderFVI++ {
		for receiverFVI := int64(3); receiverFVI <= 20; receiverFVI++ {
			if senderFVI == receiverFVI {
				continue
			}
			for senderIFV := int64(1); senderIFV <= min(senderFVI, receiverFVI); senderIFV++ {
				for receiverIFV := int64(1); receiverIFV <= min(senderFVI, receiverFVI); receiverIFV++ {
					if senderIFV == receiverIFV {
						continue
					}
					rewriter := NamespaceFailoverVersionRewriter{
						sender:   namespaceRewriteEndpoint{fvi: senderFVI, ifv: senderIFV},
						receiver: namespaceRewriteEndpoint{fvi: receiverFVI, ifv: receiverIFV},
					}
					for after := int64(1); after <= 100; after += 11 {
						got, reason := rewriter.nextSafeVersion(after)
						want, wantReason := referenceNamespaceRewrite(after, rewriter.sender, rewriter.receiver)
						require.Equal(t, wantReason, reason)
						require.Equal(t, want, got)
					}
				}
			}
		}
	}

	_, ok := namespaceRewriteNextVersion(math.MaxInt64, 10, 2)
	require.False(t, ok)
}

func TestNamespaceFailoverVersionRepeatedHandoversAdvance(t *testing.T) {
	localToRemote, remoteToLocal := testNamespaceRewriters(t, config.NamespaceFailoverVersionRewriteModeEnforce, config.StringTranslator{})
	localVersion, remoteVersion := int64(8), int64(8)
	for range 12 {
		native, ok := namespaceRewriteNextVersion(localVersion, 7, 2)
		require.True(t, ok)
		translated, reason := localToRemote.nextSafeVersion(native)
		require.Empty(t, reason)
		require.Greater(t, translated, remoteVersion)
		require.EqualValues(t, 2, namespaceRewriteLane(translated, 10))
		localVersion, remoteVersion = native, translated

		native, ok = namespaceRewriteNextVersion(remoteVersion, 10, 1)
		require.True(t, ok)
		translated, reason = remoteToLocal.nextSafeVersion(native)
		require.Empty(t, reason)
		require.Greater(t, translated, localVersion)
		require.EqualValues(t, 1, namespaceRewriteLane(translated, 7))
		remoteVersion, localVersion = native, translated
	}
}

func referenceNamespaceRewrite(after int64, sender, receiver namespaceRewriteEndpoint) (int64, string) {
	candidate, ok := namespaceRewriteNextVersion(after, receiver.fvi, receiver.ifv)
	if !ok {
		return 0, "overflow"
	}
	seen := make(map[int64]struct{})
	for {
		candidateLane := namespaceRewriteLane(candidate, sender.fvi)
		if _, exists := seen[candidateLane]; exists {
			return 0, "no_candidate"
		}
		if candidateLane != sender.ifv && candidateLane != receiver.ifv {
			return candidate, ""
		}
		seen[candidateLane] = struct{}{}
		if candidate > math.MaxInt64-receiver.fvi {
			return 0, "overflow"
		}
		candidate += receiver.fvi
	}
}

func (s *adminserviceSuite) TestNamespaceFailoverVersionInterception() {
	localToRemote, _ := testNamespaceRewriters(s.T(), config.NamespaceFailoverVersionRewriteModeEnforce, config.StringTranslator{})
	server := s.newAdminServiceProxyServer(adminProxyServerInput{
		overrides: AdminServiceOverrides{NamespaceFailoverVersionRewriter: localToRemote},
	}, &ReplicationStreamObserver{})
	request := &adminservicepb.GetNamespaceRequest{}
	upstream := proxyNamespaceResponse("migration", "cloud", 9, 9)
	s.adminClientMock.EXPECT().GetNamespace(gomock.Any(), request).Return(upstream, nil)
	counter := metrics.NamespaceFailoverVersionRewrite.WithLabelValues(
		"test", namespaceRewriteDirectionLocalToRemote, namespaceRewriteOperationGetNamespace, namespaceRewriteResultRewritten,
	)
	before := testutil.ToFloat64(counter)

	response, err := server.GetNamespace(context.Background(), request)
	s.NoError(err)
	s.EqualValues(12, response.GetFailoverVersion())
	s.Equal(before+1, testutil.ToFloat64(counter))
}

func proxyNamespaceResponse(namespace, active string, version, historyVersion int64) *adminservicepb.GetNamespaceResponse {
	return &adminservicepb.GetNamespaceResponse{
		Info: &namespacepb.NamespaceInfo{Name: namespace},
		ReplicationConfig: &replicationpb.NamespaceReplicationConfig{
			ActiveClusterName: active,
			Clusters: []*replicationpb.ClusterReplicationConfig{
				{ClusterName: "self-hosted"}, {ClusterName: "cloud"},
			},
		},
		FailoverVersion: version,
		FailoverHistory: []*replicationpb.FailoverStatus{
			{FailoverVersion: 8}, {FailoverVersion: historyVersion},
		},
	}
}

func proxyNamespaceTask(namespace, active string, version, historyVersion int64) *replicationspb.ReplicationTask {
	response := proxyNamespaceResponse(namespace, active, version, historyVersion)
	return &replicationspb.ReplicationTask{
		Attributes: &replicationspb.ReplicationTask_NamespaceTaskAttributes{
			NamespaceTaskAttributes: &replicationspb.NamespaceTaskAttributes{
				Info: response.Info, ReplicationConfig: response.ReplicationConfig,
				FailoverVersion: response.FailoverVersion, FailoverHistory: response.FailoverHistory,
			},
		},
	}
}
