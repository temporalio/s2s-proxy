//go:build testcompatibility

package topology

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	adminservicev1 "go.temporal.io/server/api/adminservice/v1"

	"github.com/temporalio/s2s-proxy/tests/compatibility/specifications"
)

const (
	cleanupTimeout                 = 90 * time.Second
	addRemoteClusterRequestTimeout = 15 * time.Second
	addRemoteClusterRetryTimeout   = 180 * time.Second
	addRemoteClusterRetryInterval  = 3 * time.Second
)

type topologyImpl struct {
	t        *testing.T
	clusters []ClusterHandle
	proxies  []ProxyHandle
}

func (top *topologyImpl) Clusters() []ClusterHandle { return top.clusters }
func (top *topologyImpl) Proxies() []ProxyHandle    { return top.proxies }
func (top *topologyImpl) T() *testing.T             { return top.t }

func (top *topologyImpl) DumpLogs() {
	for _, cluster := range top.clusters {
		for _, line := range cluster.Logs() {
			top.t.Logf("[%s] %s", cluster.Name(), line)
		}
	}
	for _, proxy := range top.proxies {
		for _, line := range proxy.Logs() {
			top.t.Logf("[%s] %s", proxy.Name(), line)
		}
	}
}

func (top *topologyImpl) Teardown(ctx context.Context) {

	// Step 1. Stop proxies (concurrently).
	var pwg sync.WaitGroup
	for _, proxy := range top.proxies {
		proxy := proxy
		pwg.Add(1)
		go func() {
			defer pwg.Done()
			_ = proxy.Stop(ctx)
		}()
	}
	pwg.Wait()

	// Step 2. Stop clusters (concurrently).
	var cwg sync.WaitGroup
	for _, cluster := range top.clusters {
		cluster := cluster
		cwg.Add(1)
		go func() {
			defer cwg.Done()
			_ = cluster.Stop(ctx)
		}()
	}
	cwg.Wait()
}

// NewTopology starts all clusters, proxies and network, connects them together, and returns a Topology.
func NewTopology(t *testing.T, topologyID string, specs []specifications.ClusterSpec) Topology {
	t.Helper()
	ctx := context.Background()

	top := &topologyImpl{t: t}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
		defer cancel()
		if t.Failed() {
			top.DumpLogs()
		}
		top.Teardown(cleanupCtx)
	})

	// Step 1. Network: Create a docker network.
	net, err := newNetwork(ctx)
	if err != nil {
		t.Fatalf("create docker network: %v", err)
	}

	// Step 2a. Clusters: Start all clusters in parallel.
	type clusterResult struct {
		idx     int
		cluster ClusterHandle
		err     error
	}
	ch := make(chan clusterResult, len(specs))

	for i, spec := range specs {
		i, spec := i, spec
		label := string(rune('a' + i)) // "a", "b", ...
		go func() {
			c, startErr := startCluster(ctx, spec, net, label, topologyID)
			ch <- clusterResult{idx: i, cluster: c, err: startErr}
		}()
	}

	// Step 2b. Clusters: Wait for all clusters to start.
	clusterHandles := make([]ClusterHandle, len(specs))
	for range specs {
		result := <-ch
		if result.err != nil {
			t.Fatalf("start cluster-%s: %v", string(rune('a'+result.idx)), result.err)
		}
		clusterHandles[result.idx] = result.cluster
	}
	top.clusters = clusterHandles

	// Step 3a. Proxies: Build proxy image.
	imageTag, err := ensureProxyImage()
	if err != nil {
		t.Fatalf("build proxy image: %v", err)
	}

	// Step 3b. Proxies: Build proxy specs (full-mesh, fully rendered).
	proxySpecs := buildProxySpecs(clusterHandles, imageTag, net)

	// Step 3c. Proxies: Start all proxies in parallel.
	type proxyChanResult struct {
		idx    int
		handle ProxyHandle
		err    error
	}
	pch := make(chan proxyChanResult, len(clusterHandles))
	for i := range clusterHandles {
		i := i
		go func() {
			handle, startErr := startProxy(ctx, proxySpecs[i], topologyID, string(rune('a'+i)))
			pch <- proxyChanResult{idx: i, handle: handle, err: startErr}
		}()
	}

	// Step 3d. Proxies: Wait for all proxies to start.
	proxyHandles := make([]ProxyHandle, len(clusterHandles))
	for range clusterHandles {
		result := <-pch
		if result.err != nil {
			t.Fatalf("start %s: %v", proxySpecs[result.idx].name, result.err)
		}
		proxyHandles[result.idx] = result.handle
	}
	top.proxies = proxyHandles

	// Step 4. Connectivity: Register each remote cluster via proxies.
	req := require.New(t)

	for i, cluster := range top.clusters {
		cluster := cluster
		proxy := top.proxies[i]
		for _, proxyAddr := range proxy.InternalAddrs() {
			proxyAddr := proxyAddr
			req.Eventually(func() bool {
				reqCtx, cancel := context.WithTimeout(ctx, addRemoteClusterRequestTimeout)
				defer cancel()
				_, err := cluster.AdminClient().AddOrUpdateRemoteCluster(reqCtx, &adminservicev1.AddOrUpdateRemoteClusterRequest{
					FrontendAddress:               proxyAddr,
					EnableRemoteClusterConnection: true,
				})
				if err != nil {
					t.Logf("AddOrUpdateRemoteCluster %s via %s: %v", cluster.Name(), proxyAddr, err)
				}
				return err == nil
			}, addRemoteClusterRetryTimeout, addRemoteClusterRetryInterval, "configure %s via %s: timed out", cluster.Name(), proxyAddr)
		}
	}

	return top
}
