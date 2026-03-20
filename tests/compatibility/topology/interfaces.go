//go:build testcompatibility

package topology

import (
	"context"
	"testing"

	workflowservicev1 "go.temporal.io/api/workflowservice/v1"
	adminservicev1 "go.temporal.io/server/api/adminservice/v1"
)

// Network is a Docker bridge network scoped to one test combination.
type Network struct {
	Name string
}

// ConfigFile pairs rendered file content with its container mount path.
type ConfigFile struct {
	Content   string
	MountPath string
}

// ClusterHandle is what tests use to interact with a running Temporal cluster.
type ClusterHandle interface {
	// Name returns the Temporal cluster name, e.g. "cluster-a".
	Name() string
	// HostAddr returns "127.0.0.1:<port>" - the test process dials this.
	HostAddr() string
	// InternalAddr returns "alias:7233" - proxy containers use this on the Docker network.
	InternalAddr() string
	// FrontendClient returns a gRPC client for the WorkflowService.
	FrontendClient() workflowservicev1.WorkflowServiceClient
	// AdminClient returns a gRPC client for the AdminService.
	AdminClient() adminservicev1.AdminServiceClient
	// Logs returns buffered log lines from the Temporal container.
	Logs() []string
	// Stop terminates all containers backing this cluster.
	Stop(ctx context.Context) error
}

// ProxyHandle is a handle to a single running proxy container.
type ProxyHandle interface {
	// Name returns the proxy name, e.g. "proxy-a" or "proxy-b".
	Name() string
	// InternalAddrs returns Docker-network addresses for all cluster connections.
	InternalAddrs() []string
	// HostAddr returns "127.0.0.1:<port>" - the test process dials this.
	HostAddr() string
	// FrontendClient returns a gRPC client for the WorkflowService via this proxy.
	FrontendClient() workflowservicev1.WorkflowServiceClient
	// Logs returns buffered log lines from the proxy container.
	Logs() []string
	// Stop terminates the proxy container.
	Stop(ctx context.Context) error
}

// Topology is the full test topology that suites operate on.
type Topology interface {
	Clusters() []ClusterHandle
	Proxies() []ProxyHandle
	T() *testing.T
	// DumpLogs emits all container logs to t.Log(). Callers decide when to invoke it.
	DumpLogs()
	// Teardown stops all containers and removes the Docker network.
	Teardown(ctx context.Context)
}
