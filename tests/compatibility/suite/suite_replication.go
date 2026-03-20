//go:build testcompatibility

package suite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	commonv1 "go.temporal.io/api/common/v1"
	taskqueuev1 "go.temporal.io/api/taskqueue/v1"
	workflowservicev1 "go.temporal.io/api/workflowservice/v1"

	"github.com/temporalio/s2s-proxy/tests/compatibility/topology"
)

type ReplicationSuite struct {
	BaseSuite
	active topology.ClusterHandle
}

func (s *ReplicationSuite) TestNamespaceReplication() {
	ctx := context.Background()

	// Step 1: Register a global namespace with s.active as the initial active cluster.
	ns := s.Ops().RegisterNamespace(ctx, s.active, "compatibility-ns")

	// Step 2: Validate that the active cluster for this namespace is the one we passed in.
	active := s.Ops().Active(ns)
	s.Require().NotNil(active)
	s.Require().Equal(s.active.Name(), active.Name())

	// Step 3: Verify namespace has replicated to all passive clusters.
	for _, passive := range s.Ops().Passives(ns) {
		s.Ops().WaitForNamespace(passive, ns)
	}
}

func (s *ReplicationSuite) TestWorkflowReplication() {
	ctx := context.Background()

	// Step 1: Register a global namespace with s.active as the initial active cluster.
	ns := s.Ops().RegisterNamespace(ctx, s.active, "compatibility-wf-ns")

	// Step 2: Validate that the active cluster for this namespace is the one we passed in.
	active := s.Ops().Active(ns)
	s.Require().NotNil(active)
	s.Require().Equal(s.active.Name(), active.Name())

	// Step 3: Wait for namespace replication to all passive clusters.
	passives := s.Ops().Passives(ns)
	for _, c := range passives {
		s.Ops().WaitForNamespace(c, ns)
	}

	// Step 4: Start a workflow on the active cluster.
	workflowID := fmt.Sprintf("compatibility-wf-%s-%d", s.active.Name(), time.Now().UnixNano())
	startCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()
	_, err := s.active.FrontendClient().StartWorkflowExecution(startCtx,
		&workflowservicev1.StartWorkflowExecutionRequest{
			Namespace:    ns,
			WorkflowId:   workflowID,
			WorkflowType: &commonv1.WorkflowType{Name: "compatibility-wf"},
			TaskQueue:    &taskqueuev1.TaskQueue{Name: "compatibility-tq"},
			RequestId:    workflowID + "-start",
		})
	s.Require().NoError(err, "start workflow on %s", s.active.Name())

	// Step 5: Verify workflow history has replicated to all passive clusters.
	s.Ops().WaitForWorkflowVisible(ctx, passives, ns, workflowID)

	// Step 6: Terminate the workflow on the active cluster.
	s.Ops().TerminateWorkflow(ctx, s.active, ns, workflowID)

	// Step 7: Verify termination has replicated to all passive clusters.
	s.Ops().WaitForWorkflowTerminated(ctx, passives, ns, workflowID)
}

// RunReplicationSuite runs the ReplicationSuite with the given active cluster against the provided Env.
func RunReplicationSuite(t *testing.T, top topology.Topology, active topology.ClusterHandle) {
	s := &ReplicationSuite{BaseSuite: BaseSuite{top: top}, active: active}
	suite.Run(t, s)
}
