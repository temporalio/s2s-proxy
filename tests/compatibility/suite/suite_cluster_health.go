//go:build testcompatibility

package suite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"

	"github.com/temporalio/s2s-proxy/tests/compatibility/topology"
)

type ClusterHealthSuite struct {
	BaseSuite
	cluster topology.ClusterHandle
}

func (s *ClusterHealthSuite) TestGetSystemInfo() {
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	// Step 1: GetSystemInfo - verifies gRPC connectivity and basic Temporal responsiveness.
	s.T().Logf("checking cluster %s at %s", s.cluster.Name(), s.cluster.HostAddr())
	sysResp, err := s.cluster.FrontendClient().GetSystemInfo(ctx, &workflowservicev1.GetSystemInfoRequest{})
	s.Require().NoError(err, "GetSystemInfo on %s", s.cluster.Name())
	s.T().Logf("cluster %s: Temporal server version=%s", s.cluster.Name(), sysResp.GetServerVersion())
}

func (s *ClusterHealthSuite) TestListNamespaces() {
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	// Step 1: ListNamespaces - verifies the namespace endpoint is healthy.
	_, err := s.cluster.FrontendClient().ListNamespaces(ctx, &workflowservicev1.ListNamespacesRequest{})
	s.Require().NoError(err, "ListNamespaces on %s", s.cluster.Name())
}

func RunClusterHealthSuite(t *testing.T, top topology.Topology, cluster topology.ClusterHandle) {
	s := &ClusterHealthSuite{BaseSuite: BaseSuite{top: top}, cluster: cluster}
	suite.Run(t, s)
}
