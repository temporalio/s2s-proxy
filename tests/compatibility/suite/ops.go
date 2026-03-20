//go:build testcompatibility

package suite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	commonv1 "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	replicationv1 "go.temporal.io/api/replication/v1"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/temporalio/s2s-proxy/tests/compatibility/topology"
)

// Ops provides a slew of test operations for making changes and assertions across the topology
type Ops struct {
	t   testing.TB
	top topology.Topology
}

// Active returns the cluster where ns is active.
func (ops Ops) Active(ns string) topology.ClusterHandle {
	for _, c := range ops.top.Clusters() {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		resp, err := c.FrontendClient().DescribeNamespace(ctx, &workflowservicev1.DescribeNamespaceRequest{Namespace: ns})
		cancel()
		if err != nil {
			continue
		}
		if resp.GetReplicationConfig().GetActiveClusterName() == c.Name() {
			return c
		}
	}
	return nil
}

// Passives returns the clusters where ns is not active.
func (ops Ops) Passives(ns string) []topology.ClusterHandle {
	active := ops.Active(ns)
	if active == nil {
		return nil
	}
	var passives []topology.ClusterHandle
	for _, c := range ops.top.Clusters() {
		if c.Name() != active.Name() {
			passives = append(passives, c)
		}
	}
	return passives
}

// RegisterNamespace registers a global namespace with the provided name on target.
func (ops Ops) RegisterNamespace(
	ctx context.Context,
	target topology.ClusterHandle,
	base string,
) string {

	// Step 1. Create a deduped namespace name (in case of parallel test runs).
	ns := fmt.Sprintf("%s-%d", base, time.Now().UnixNano()%1_000_000)

	// Step 2. Register the namespace on target, including all clusters in the replication config.
	var clusterConfigs []*replicationv1.ClusterReplicationConfig
	for _, c := range ops.top.Clusters() {
		clusterConfigs = append(clusterConfigs, &replicationv1.ClusterReplicationConfig{
			ClusterName: c.Name(),
		})
	}
	require.New(ops.t).Eventually(func() bool {
		nsCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
		defer cancel()
		_, err := target.FrontendClient().RegisterNamespace(nsCtx, &workflowservicev1.RegisterNamespaceRequest{
			Namespace:                        ns,
			IsGlobalNamespace:                true,
			WorkflowExecutionRetentionPeriod: durationpb.New(72 * time.Hour),
			Clusters:                         clusterConfigs,
			ActiveClusterName:                target.Name(),
		})
		if err != nil {
			ops.t.Logf("RegisterNamespace attempt failed (expected if metadata not yet propagated): %v", err)
		}
		return err == nil
	}, registerNamespaceTimeout, registerNamespaceInterval,
		"register global namespace %s on %s", ns, target.Name())
	return ns
}

// WaitForNamespace polls cluster until ns is visible.
func (ops Ops) WaitForNamespace(cluster topology.ClusterHandle, ns string) {
	require.New(ops.t).Eventually(func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		defer cancel()
		resp, err := cluster.FrontendClient().DescribeNamespace(ctx,
			&workflowservicev1.DescribeNamespaceRequest{Namespace: ns})
		return err == nil && resp.GetNamespaceInfo().GetName() == ns
	}, replicationTimeout, replicationInterval,
		"namespace %s should replicate to %s", ns, cluster.Name())
}

// WaitForWorkflowVisible polls each passive cluster until workflowID is visible in ns.
func (ops Ops) WaitForWorkflowVisible(ctx context.Context, passives []topology.ClusterHandle, ns, wfID string) {
	for _, passive := range passives {
		passive := passive
		require.New(ops.t).Eventually(func() bool {
			return isWorkflowVisibleOn(ctx, passive, ns, wfID)
		}, replicationTimeout, replicationInterval,
			"workflow should replicate to %s", passive.Name())
	}
}

// WaitForWorkflowTerminated asserts Eventually that workflowID has TERMINATED status on each cluster in passives.
func (ops Ops) WaitForWorkflowTerminated(ctx context.Context, passives []topology.ClusterHandle, ns, wfID string) {
	for _, passive := range passives {
		passive := passive
		require.New(ops.t).Eventually(func() bool {
			return isWorkflowTerminatedOn(ctx, passive, ns, wfID)
		}, replicationTimeout, replicationInterval,
			"TERMINATED should replicate to %s", passive.Name())
	}
}

// TerminateWorkflow terminates wfID on cluster.
func (ops Ops) TerminateWorkflow(ctx context.Context, cluster topology.ClusterHandle, ns, wfID string) {
	termCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_, err := cluster.FrontendClient().TerminateWorkflowExecution(termCtx, &workflowservicev1.TerminateWorkflowExecutionRequest{
		Namespace:         ns,
		WorkflowExecution: &commonv1.WorkflowExecution{WorkflowId: wfID},
		Reason:            "compatibility test complete",
	})
	require.New(ops.t).NoError(err, "terminate workflow on %s", cluster.Name())
}

// SetActive issues UpdateNamespace to fail ns over to target, then polls Active() until target confirms it is the active cluster.
func (ops Ops) SetActive(ns string, target topology.ClusterHandle) {
	failCtx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()
	_, err := target.FrontendClient().UpdateNamespace(failCtx, &workflowservicev1.UpdateNamespaceRequest{
		Namespace: ns,
		ReplicationConfig: &replicationv1.NamespaceReplicationConfig{
			ActiveClusterName: target.Name(),
		},
	})
	require.New(ops.t).NoError(err, "SetActive: UpdateNamespace to %s", target.Name())

	require.New(ops.t).Eventually(func() bool {
		active := ops.Active(ns)
		return active != nil && active.Name() == target.Name()
	}, replicationTimeout, replicationInterval,
		"namespace %s should become active on %s", ns, target.Name())
}

func isWorkflowVisibleOn(ctx context.Context, cluster topology.ClusterHandle, ns, wfID string) bool {
	rCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	resp, err := cluster.FrontendClient().DescribeWorkflowExecution(rCtx, &workflowservicev1.DescribeWorkflowExecutionRequest{
		Namespace: ns,
		Execution: &commonv1.WorkflowExecution{WorkflowId: wfID},
	})
	return err == nil && resp.GetWorkflowExecutionInfo() != nil
}

func isWorkflowTerminatedOn(ctx context.Context, cluster topology.ClusterHandle, ns, wfID string) bool {
	rCtx, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	resp, err := cluster.FrontendClient().DescribeWorkflowExecution(rCtx, &workflowservicev1.DescribeWorkflowExecutionRequest{
		Namespace: ns,
		Execution: &commonv1.WorkflowExecution{WorkflowId: wfID},
	})
	if err != nil {
		return false
	}
	return resp.GetWorkflowExecutionInfo().GetStatus() == enumspb.WORKFLOW_EXECUTION_STATUS_TERMINATED
}
