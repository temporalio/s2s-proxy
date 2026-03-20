//go:build testcompatibility

package suite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"

	"github.com/temporalio/s2s-proxy/tests/compatibility/topology"
)

type ConnectivitySuite struct {
	BaseSuite
	proxy topology.ProxyHandle
}

func (s *ConnectivitySuite) TestProxyForwarding() {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	// Step 1: GetSystemInfo via proxy - verifies the proxy forwards gRPC requests to the backend cluster.
	resp, err := s.proxy.FrontendClient().GetSystemInfo(ctx, &workflowservicev1.GetSystemInfoRequest{})
	s.Require().NoError(err)

	// Step 2: Verify capabilities - confirms a live Temporal response was returned through the proxy.
	s.Require().NotNil(resp.GetCapabilities())
}

func (s *ConnectivitySuite) TestListNamespacesViaProxy() {
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()

	// Step 1: ListNamespaces via proxy - verifies the namespace endpoint is reachable through the proxy.
	resp, err := s.proxy.FrontendClient().ListNamespaces(ctx, &workflowservicev1.ListNamespacesRequest{})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
}

func RunConnectivitySuite(t *testing.T, top topology.Topology, proxy topology.ProxyHandle) {
	s := &ConnectivitySuite{BaseSuite: BaseSuite{top: top}, proxy: proxy}
	suite.Run(t, s)
}
