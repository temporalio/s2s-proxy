package cadencetype

import (
	"fmt"
	cadenceadmin "github.com/uber/cadence-idl/go/proto/admin/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	temporalreplication "go.temporal.io/server/api/replication/v1"
)

func ReplicationMessages(v *temporalreplication.ReplicationMessages) *cadenceadmin.ReplicationMessages {
	if v == nil {
		return nil
	}

	return &cadenceadmin.ReplicationMessages{
		ReplicationTasks:       ReplicationTasks(v.GetReplicationTasks()),
		LastRetrievedMessageId: v.GetLastRetrievedMessageId(),
		HasMore:                v.GetHasMore(),
		SyncShardStatus:        SyncShardStatus(v.GetSyncShardStatus()),
	}
}

func SyncShardStatus(status *temporalreplication.SyncShardStatus) *cadenceadmin.SyncShardStatus {
	if status == nil {
		return nil
	}

	return &cadenceadmin.SyncShardStatus{
		Timestamp: Timestamp(status.GetStatusTime()),
	}
}

func ReplicationTasks(tasks []*temporalreplication.ReplicationTask) []*cadenceadmin.ReplicationTask {
	if tasks == nil {
		return nil
	}

	result := make([]*cadenceadmin.ReplicationTask, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, ReplicationTask(t))
	}
	return result
}

// ReplicationTask is only used in domain replication.
func ReplicationTask(t *temporalreplication.ReplicationTask) *cadenceadmin.ReplicationTask {
	if t == nil {
		return nil
	}

	replicationTask := &cadenceadmin.ReplicationTask{
		SourceTaskId: t.GetSourceTaskId(),
		CreationTime: Timestamp(t.GetVisibilityTime()),
	}

	switch t.GetTaskType() {
	case enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK:
		replicationTask.TaskType = cadenceadmin.ReplicationTaskType_REPLICATION_TASK_TYPE_DOMAIN
		replicationTask.Attributes = DomainTaskAttributes(t.GetNamespaceTaskAttributes())
	default:
		panic(fmt.Sprintf("unsupported replication task type: %v", t.GetTaskType()))
	}

	return replicationTask
}

func DomainTaskAttributes(attributes *temporalreplication.NamespaceTaskAttributes) *cadenceadmin.ReplicationTask_DomainTaskAttributes {
	if attributes == nil {
		return nil
	}

	return &cadenceadmin.ReplicationTask_DomainTaskAttributes{
		DomainTaskAttributes: &cadenceadmin.DomainTaskAttributes{
			DomainOperation: DomainOperation(attributes.GetNamespaceOperation()),
			Id:              attributes.GetId(),
			Domain:          Domain(attributes.GetInfo(), attributes.GetConfig(), attributes.GetReplicationConfig(), attributes.GetFailoverVersion()),
			ConfigVersion:   attributes.GetConfigVersion(),
			FailoverVersion: attributes.GetFailoverVersion(),
			//PreviousFailoverVersion: 0,
		},
	}
}
