package interceptor

import (
	"testing"

	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/replication/v1"
	"go.temporal.io/server/api/adminservice/v1"
)

func generateClusterNameObjs() []objCase {
	return []objCase{
		{
			objName: "nil",
			makeType: func(clusterName string) any {
				return nil
			},
		},
		{
			objName: "DescribeClusterResponse",
			makeType: func(clusterName string) any {
				return &adminservice.DescribeClusterResponse{
					ServerVersion:     "1.2.3",
					ClusterId:         "abc123",
					ClusterName:       clusterName,
					HistoryShardCount: 1234,
				}
			},
		},
		{
			objName:         "GetNamespaceResponse",
			containsObjName: false,
			makeType: func(name string) any {
				return &adminservice.GetNamespaceResponse{
					Info: &namespace.NamespaceInfo{
						Name: "namespace-name",
					},
					Config: &namespace.NamespaceConfig{},
					ReplicationConfig: &replication.NamespaceReplicationConfig{
						ActiveClusterName: name,
						Clusters: []*replication.ClusterReplicationConfig{
							{
								ClusterName: name,
							},
							{

								ClusterName: "some-other-name",
							},
						},
					},
					ConfigVersion:     1,
					FailoverVersion:   2,
					FailoverHistory:   []*replication.FailoverStatus{},
					IsGlobalNamespace: true,
				}

			},
			expError: "",
		},
	}
}

func TestTranslateClusterName(t *testing.T) {
	testTranslateObjects(t, generateClusterNameObjs())
}
