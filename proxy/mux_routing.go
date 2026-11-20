package proxy

import (
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

func isReplicationMuxMethod(fullMethodName string) bool {
	switch fullMethodName {
	case adminservice.AdminService_StreamWorkflowReplicationMessages_FullMethodName,
		adminservice.AdminService_GetReplicationMessages_FullMethodName,
		adminservice.AdminService_GetNamespaceReplicationMessages_FullMethodName,
		adminservice.AdminService_GetDLQReplicationMessages_FullMethodName,
		adminservice.AdminService_ResendReplicationTasks_FullMethodName,
		adminservice.AdminService_GenerateLastHistoryReplicationTasks_FullMethodName,
		adminservice.AdminService_SyncWorkflowState_FullMethodName,
		adminservice.AdminService_ReapplyEvents_FullMethodName:
		return true
	default:
		return false
	}
}

func newMuxPoolObserver(name string, direction string, logger log.Logger) func(grpcutil.PoolSnapshot) {
	return func(snapshot grpcutil.PoolSnapshot) {
		observeMuxPool(name, direction, grpcutil.GeneralPoolName, snapshot.General)
		observeMuxPool(name, direction, grpcutil.ReplicationPoolName, snapshot.Replication)
		if logger != nil {
			logger.Info("Updated mux traffic pools",
				tag.Name(name),
				tag.NewStringTag("direction", direction),
				tag.NewInt("generalTarget", snapshot.General.Target),
				tag.NewInt("generalAssigned", snapshot.General.Assigned),
				tag.NewInt("replicationTarget", snapshot.Replication.Target),
				tag.NewInt("replicationAssigned", snapshot.Replication.Assigned))
		}
	}
}

func observeMuxPool(name string, direction string, pool string, status grpcutil.PoolStatus) {
	labels := []string{name, direction, pool}
	metrics.MuxPoolSessionsTarget.WithLabelValues(labels...).Set(float64(status.Target))
	metrics.MuxPoolSessionsAssigned.WithLabelValues(labels...).Set(float64(status.Assigned))
}
