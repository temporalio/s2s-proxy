//go:build testcompatibility

package matrix

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/temporalio/s2s-proxy/tests/compatibility/specifications"
	"github.com/temporalio/s2s-proxy/tests/compatibility/suite"
	"github.com/temporalio/s2s-proxy/tests/compatibility/topology"
)

// run runs the full test suite for a single topology of cluster specs.
func run(t *testing.T, specs []specifications.ClusterSpec) {
	t.Helper()

	top := topology.NewTopology(t, newTopologyID(), specs)

	healthPassed := t.Run("Health", func(t *testing.T) {
		for _, cluster := range top.Clusters() {
			if !t.Run(cluster.Name(), func(t *testing.T) { suite.RunClusterHealthSuite(t, top, cluster) }) {
				return
			}
		}

		for _, proxy := range top.Proxies() {
			if !t.Run(proxy.Name(), func(t *testing.T) { suite.RunProxyHealthSuite(t, top, proxy) }) {
				return
			}
		}
	})

	// If health checks fail, we skip the rest of the tests to avoid misleading failures.
	// If health / Proxy tests fail, this is likely indication of cluster configuration issues.
	if !healthPassed {
		return
	}

	t.Run("Connectivity", func(t *testing.T) {
		for _, proxy := range top.Proxies() {
			t.Run(proxy.Name(), func(t *testing.T) {
				suite.RunConnectivitySuite(t, top, proxy)
			})
		}
	})

	t.Run("Replication", func(t *testing.T) {
		for _, cluster := range top.Clusters() {
			t.Run(cluster.Name(), func(t *testing.T) {
				suite.RunReplicationSuite(t, top, cluster)
			})
		}
	})
}

// newTopologyID generates a random topology ID for test isolation.
// Used to separate parallel topology runs in the global Docker runtime.
// For example, to ensure that the "cluster-a" in "topology-1" and "topology-2" are isolated.
func newTopologyID() string {
	return fmt.Sprintf("%08x", rand.Uint32()) //nolint:gosec
}
