//go:build testcompatibility

package suite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"

	"github.com/temporalio/s2s-proxy/tests/compatibility/topology"
)

type ProxyHealthSuite struct {
	BaseSuite
	proxy topology.ProxyHandle
}

func (s *ProxyHealthSuite) TestGetSystemInfo() {
	ctx, cancel := context.WithTimeout(context.Background(), healthCheckTimeout)
	defer cancel()

	// Step 1: GetSystemInfo - verifies gRPC connectivity through the proxy.
	s.T().Logf("checking %s at %s", s.proxy.Name(), s.proxy.HostAddr())
	resp, err := s.proxy.FrontendClient().GetSystemInfo(ctx, &workflowservicev1.GetSystemInfoRequest{})
	s.Require().NoError(err, "GetSystemInfo via %s", s.proxy.Name())

	// Step 2: Verify capabilities - confirms the proxy is forwarding a live Temporal response.
	s.Require().NotNil(resp.GetCapabilities(), "%s: GetSystemInfo returned nil capabilities", s.proxy.Name())
	s.T().Logf("%s: reachable, server version=%s", s.proxy.Name(), resp.GetServerVersion())
}

func RunProxyHealthSuite(t *testing.T, top topology.Topology, proxy topology.ProxyHandle) {
	s := &ProxyHealthSuite{BaseSuite: BaseSuite{top: top}, proxy: proxy}
	suite.Run(t, s)
}
