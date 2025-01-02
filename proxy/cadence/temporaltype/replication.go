package temporaltype

import (
	"fmt"
	cadenceadmin "github.com/uber/cadence-idl/go/proto/admin/v1"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/replication/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/api/history/v1"
	repication "go.temporal.io/server/api/replication/v1"
)

func ReplicationTasks(tasks []*cadenceadmin.ReplicationTask) []*repication.ReplicationTask {
	if tasks == nil {
		return nil
	}

	result := make([]*repication.ReplicationTask, len(tasks))
	for i, task := range tasks {
		result[i] = ReplicationTask(task)
	}
	return result
}

func ReplicationTask(t *cadenceadmin.ReplicationTask) *repication.ReplicationTask {
	task := &repication.ReplicationTask{
		SourceTaskId:   t.GetSourceTaskId(),
		VisibilityTime: Timestamp(t.GetCreationTime()),
	}

	switch t.GetTaskType() {
	case cadenceadmin.ReplicationTaskType_REPLICATION_TASK_TYPE_HISTORY_V2:
		task.TaskType = enumsspb.REPLICATION_TASK_TYPE_HISTORY_V2_TASK
		task.Attributes = HistoryTaskAttributes(t.GetHistoryTaskV2Attributes())
	case cadenceadmin.ReplicationTaskType_REPLICATION_TASK_TYPE_DOMAIN:
		task.TaskType = enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK
		task.Attributes = NamespaceTaskAttributes(t.GetDomainTaskAttributes())
	default:
		panic(fmt.Sprintf("unknown Cadence replication task type: %v", t.GetTaskType()))
	}

	return task
}

func HistoryTaskAttributes(attributes *cadenceadmin.HistoryTaskV2Attributes) *repication.ReplicationTask_HistoryTaskAttributes {
	return &repication.ReplicationTask_HistoryTaskAttributes{
		HistoryTaskAttributes: &repication.HistoryTaskAttributes{
			NamespaceId:         attributes.GetDomainId(),
			WorkflowId:          attributes.GetWorkflowExecution().GetWorkflowId(),
			RunId:               attributes.GetWorkflowExecution().GetRunId(),
			VersionHistoryItems: VersionHistoryItems(attributes.GetVersionHistoryItems()),
			Events:              Events(attributes.GetEvents()),
			NewRunEvents:        Events(attributes.GetNewRunEvents()),
			//BaseExecutionInfo:   nil,
			//NewRunId:            "",
		},
	}
}

func VersionHistoryItems(items []*cadenceadmin.VersionHistoryItem) []*history.VersionHistoryItem {
	if items == nil {
		return nil
	}

	result := make([]*history.VersionHistoryItem, len(items))
	for i, item := range items {
		result[i] = &history.VersionHistoryItem{
			EventId: item.GetEventId(),
			Version: item.GetVersion(),
		}
	}
	return result
}

func NamespaceTaskAttributes(attributes *cadenceadmin.DomainTaskAttributes) *repication.ReplicationTask_NamespaceTaskAttributes {
	return &repication.ReplicationTask_NamespaceTaskAttributes{
		NamespaceTaskAttributes: &repication.NamespaceTaskAttributes{
			NamespaceOperation: NamespaceOperation(attributes.GetDomainOperation()),
			Id:                 attributes.GetId(),
			Info:               NamespaceInfo(attributes.GetDomain()),
			Config:             NamespaceConfig(attributes.GetDomain()),
			ReplicationConfig:  ReplicationConfig(attributes),
			ConfigVersion:      attributes.GetConfigVersion(),
			FailoverVersion:    attributes.GetFailoverVersion(),
			//FailoverHistory:       nil,
			//NexusOutgoingServices: nil,
		},
	}
}

func ReplicationConfig(attributes *cadenceadmin.DomainTaskAttributes) *replication.NamespaceReplicationConfig {
	var clusters []*replication.ClusterReplicationConfig
	for _, cluster := range attributes.GetDomain().GetClusters() {
		clusters = append(clusters, &replication.ClusterReplicationConfig{
			ClusterName: cluster.GetClusterName(),
		})
	}

	return &replication.NamespaceReplicationConfig{
		ActiveClusterName: attributes.GetDomain().GetActiveClusterName(),
		Clusters:          clusters,
		State:             enums.REPLICATION_STATE_NORMAL,
	}
}

func NamespaceConfig(domain *cadence.Domain) *namespace.NamespaceConfig {
	if domain == nil {
		return nil
	}

	return &namespace.NamespaceConfig{
		WorkflowExecutionRetentionTtl: Duration(domain.GetWorkflowExecutionRetentionPeriod()),
		BadBinaries:                   BadBinaries(domain.GetBadBinaries()),
		HistoryArchivalState:          enums.ArchivalState(domain.GetHistoryArchivalStatus()),
		HistoryArchivalUri:            domain.GetHistoryArchivalUri(),
		VisibilityArchivalState:       enums.ArchivalState(domain.GetVisibilityArchivalStatus()),
		VisibilityArchivalUri:         domain.GetVisibilityArchivalUri(),
		//CustomSearchAttributeAliases:  nil,
	}
}

func BadBinaries(binaries *cadence.BadBinaries) *namespace.BadBinaries {
	if binaries == nil {
		return nil
	}

	badBinaries := make(map[string]*namespace.BadBinaryInfo)
	for k, v := range binaries.GetBinaries() {
		badBinaries[k] = &namespace.BadBinaryInfo{
			Reason:     v.GetReason(),
			Operator:   v.GetOperator(),
			CreateTime: Timestamp(v.GetCreatedTime()),
		}
	}

	return &namespace.BadBinaries{
		Binaries: badBinaries,
	}
}

func NamespaceInfo(domain *cadence.Domain) *namespace.NamespaceInfo {
	if domain == nil {
		return nil
	}

	return &namespace.NamespaceInfo{
		Name:        domain.GetName(),
		State:       enums.NamespaceState(domain.GetStatus()),
		Description: domain.GetDescription(),
		OwnerEmail:  domain.GetOwnerEmail(),
		Data:        domain.GetData(),
		Id:          domain.GetId(),
		//Capabilities:      nil,
		//SupportsSchedules: false,
	}
}

func NamespaceOperation(operation cadenceadmin.DomainOperation) enumsspb.NamespaceOperation {
	return enumsspb.NamespaceOperation(operation)
}

func SyncShardStatus(status *cadenceadmin.SyncShardStatus) *repication.SyncShardStatus {
	if status == nil {
		return nil
	}

	return &repication.SyncShardStatus{
		StatusTime: Timestamp(status.GetTimestamp()),
	}
}

func ReplicationToken(tokens []*cadenceadmin.ReplicationToken) []*repication.ReplicationToken {
	if tokens == nil {
		return nil
	}

	result := make([]*repication.ReplicationToken, len(tokens))
	for i, token := range tokens {
		result[i] = &repication.ReplicationToken{
			ShardId:                token.GetShardId() + 1,
			LastRetrievedMessageId: token.GetLastRetrievedMessageId(),
			LastProcessedMessageId: token.GetLastProcessedMessageId(),
			//LastProcessedVisibilityTime: nil,
		}
	}
	return result
}

func VersionHistory(vh *cadenceadmin.VersionHistory) *history.VersionHistory {
	if vh == nil {
		return nil
	}

	return &history.VersionHistory{
		BranchToken: vh.GetBranchToken(),
		Items:       VersionHistoryItems(vh.GetItems()),
	}
}
