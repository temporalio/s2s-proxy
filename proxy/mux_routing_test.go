package proxy

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

func TestIsReplicationMuxMethod(t *testing.T) {
	tests := []struct {
		name   string
		method string
		want   bool
	}{
		{name: "stream replication", method: adminservice.AdminService_StreamWorkflowReplicationMessages_FullMethodName, want: true},
		{name: "legacy replication", method: adminservice.AdminService_GetReplicationMessages_FullMethodName, want: true},
		{name: "namespace replication", method: adminservice.AdminService_GetNamespaceReplicationMessages_FullMethodName, want: true},
		{name: "DLQ replication", method: adminservice.AdminService_GetDLQReplicationMessages_FullMethodName, want: true},
		{name: "resend replication", method: adminservice.AdminService_ResendReplicationTasks_FullMethodName, want: true},
		{name: "generate replication", method: adminservice.AdminService_GenerateLastHistoryReplicationTasks_FullMethodName, want: true},
		{name: "sync workflow", method: adminservice.AdminService_SyncWorkflowState_FullMethodName, want: true},
		{name: "reapply events", method: adminservice.AdminService_ReapplyEvents_FullMethodName, want: true},
		{name: "admin", method: adminservice.AdminService_DescribeCluster_FullMethodName},
		{name: "raw history", method: adminservice.AdminService_GetWorkflowExecutionRawHistoryV2_FullMethodName},
		{name: "workflow service", method: "/temporal.api.workflowservice.v1.WorkflowService/DescribeWorkflowExecution"},
		{name: "unknown", method: "/new.Service/UnknownMethod"},
		{name: "malformed", method: "Malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, isReplicationMuxMethod(tt.method))
		})
	}
}

func TestMuxPoolObserver(t *testing.T) {
	name := "observer-test"
	direction := "outbound-test"
	observer := newMuxPoolObserver(name, direction, log.NewNoopLogger())
	observer(grpcutil.PoolSnapshot{
		General:     grpcutil.PoolStatus{Target: 2, Assigned: 1},
		Replication: grpcutil.PoolStatus{Target: 8, Assigned: 7},
	})

	require.Equal(t, float64(2), testutil.ToFloat64(metrics.MuxPoolSessionsTarget.WithLabelValues(name, direction, grpcutil.GeneralPoolName)))
	require.Equal(t, float64(1), testutil.ToFloat64(metrics.MuxPoolSessionsAssigned.WithLabelValues(name, direction, grpcutil.GeneralPoolName)))
	require.Equal(t, float64(8), testutil.ToFloat64(metrics.MuxPoolSessionsTarget.WithLabelValues(name, direction, grpcutil.ReplicationPoolName)))
	require.Equal(t, float64(7), testutil.ToFloat64(metrics.MuxPoolSessionsAssigned.WithLabelValues(name, direction, grpcutil.ReplicationPoolName)))
}
