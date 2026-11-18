package proxy

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	namespacepb "go.temporal.io/api/namespace/v1"
	replicationpb "go.temporal.io/api/replication/v1"
	"go.temporal.io/server/api/adminservice/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"

	"github.com/temporalio/s2s-proxy/config"
)

func testFailoverRule() config.NamespaceFailoverVersionRule {
	return config.NamespaceFailoverVersionRule{Namespace: "prototype", TargetCluster: "cloud", SourceInitialFailoverVersion: 1, SourceIncrement: 7, TargetInitialFailoverVersion: 2, TargetIncrement: 10, MinVersion: 9}
}

func TestRewriteFailoverVersion(t *testing.T) {
	r := testFailoverRule()
	for _, tc := range []struct {
		name     string
		in, want int64
	}{
		{"cutover", 9, 12}, {"source history", 8, 8}, {"before migration", 2, 2},
		{"native target replay", 12, 12}, {"skip candidate owned by source", 16, 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := rewriteFailoverVersion(tc.in, r)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
			again, err := rewriteFailoverVersion(got, r)
			require.NoError(t, err)
			require.Equal(t, got, again)
		})
	}
	r.MinVersion = 1
	r.SourceIncrement = 1000005
	r.SourceInitialFailoverVersion = 214689
	r.TargetIncrement = 1000000
	r.TargetInitialFailoverVersion = 100000
	got, err := rewriteFailoverVersion(6100030, r)
	require.NoError(t, err)
	require.EqualValues(t, 7100000, got)
	r = testFailoverRule()
	r.SourceIncrement = 3
	r.SourceInitialFailoverVersion = 1
	r.TargetInitialFailoverVersion = 2
	r.TargetIncrement = 7
	r.MinVersion = 1
	_, err = rewriteFailoverVersion(math.MaxInt64-2, r)
	require.Error(t, err)
}

func (s *adminserviceSuite) TestNamespaceFailoverRewriteBothRetrievalPaths() {
	rule := testFailoverRule()
	makeHistory := func() []*replicationpb.FailoverStatus {
		return []*replicationpb.FailoverStatus{{FailoverVersion: 8}, {FailoverVersion: 9}}
	}
	server := s.newAdminServiceProxyServer(adminProxyServerInput{overrides: AdminServiceOverrides{NamespaceFailoverVersionRules: []config.NamespaceFailoverVersionRule{rule}}}, &ReplicationStreamObserver{})
	getResp := &adminservice.GetNamespaceResponse{Info: &namespacepb.NamespaceInfo{Name: "prototype"}, ReplicationConfig: &replicationpb.NamespaceReplicationConfig{ActiveClusterName: "cloud"}, FailoverVersion: 9, FailoverHistory: makeHistory()}
	s.adminClientMock.EXPECT().GetNamespace(context.Background(), &adminservice.GetNamespaceRequest{}).Return(getResp, nil)
	got, err := server.GetNamespace(context.Background(), &adminservice.GetNamespaceRequest{})
	s.NoError(err)
	s.EqualValues(12, got.FailoverVersion)
	s.EqualValues(8, got.FailoverHistory[0].FailoverVersion)
	s.EqualValues(12, got.FailoverHistory[1].FailoverVersion)
	attrs := &replicationspb.NamespaceTaskAttributes{Info: &namespacepb.NamespaceInfo{Name: "prototype"}, ReplicationConfig: &replicationpb.NamespaceReplicationConfig{ActiveClusterName: "cloud"}, FailoverVersion: 9, FailoverHistory: makeHistory()}
	untouched := &replicationspb.NamespaceTaskAttributes{Info: &namespacepb.NamespaceInfo{Name: "other"}, ReplicationConfig: &replicationpb.NamespaceReplicationConfig{ActiveClusterName: "cloud"}, FailoverVersion: 9}
	resp := &adminservice.GetNamespaceReplicationMessagesResponse{Messages: &replicationspb.ReplicationMessages{ReplicationTasks: []*replicationspb.ReplicationTask{{Attributes: &replicationspb.ReplicationTask_NamespaceTaskAttributes{NamespaceTaskAttributes: attrs}}, {Attributes: &replicationspb.ReplicationTask_NamespaceTaskAttributes{NamespaceTaskAttributes: untouched}}}}}
	s.adminClientMock.EXPECT().GetNamespaceReplicationMessages(context.Background(), &adminservice.GetNamespaceReplicationMessagesRequest{}).Return(resp, nil)
	_, err = server.GetNamespaceReplicationMessages(context.Background(), &adminservice.GetNamespaceReplicationMessagesRequest{})
	s.NoError(err)
	s.EqualValues(12, attrs.FailoverVersion)
	s.EqualValues(12, attrs.FailoverHistory[1].FailoverVersion)
	s.EqualValues(9, untouched.FailoverVersion)
}
