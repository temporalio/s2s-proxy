package temporaltype

import (
	cadenceadmin "github.com/uber/cadence-idl/go/proto/admin/v1"
	temporal "go.temporal.io/api/common/v1"
	temporaladmin "go.temporal.io/server/api/adminservice/v1"
	repication "go.temporal.io/server/api/replication/v1"
)

func GetReplicationMessagesResponse(resp *cadenceadmin.GetReplicationMessagesResponse) *temporaladmin.GetReplicationMessagesResponse {
	if resp == nil {
		return nil
	}

	messages := make(map[int32]*repication.ReplicationMessages, len(resp.GetShardMessages()))
	for i, m := range resp.GetShardMessages() {
		messages[i+1] = &repication.ReplicationMessages{
			ReplicationTasks:       ReplicationTasks(m.GetReplicationTasks()),
			LastRetrievedMessageId: m.GetLastRetrievedMessageId(),
			HasMore:                m.GetHasMore(),
			SyncShardStatus:        SyncShardStatus(m.GetSyncShardStatus()),
		}
	}

	return &temporaladmin.GetReplicationMessagesResponse{
		ShardMessages: messages,
	}
}

func DescribeClusterResponse(resp *cadenceadmin.DescribeClusterResponse) *temporaladmin.DescribeClusterResponse {
	if resp == nil {
		return nil
	}

	return &temporaladmin.DescribeClusterResponse{
		SupportedClients:         nil,
		ServerVersion:            "",
		MembershipInfo:           nil,
		ClusterId:                "cadence-uuid",
		ClusterName:              "cadence-2",
		HistoryShardCount:        1,
		PersistenceStore:         "",
		VisibilityStore:          "",
		VersionInfo:              nil,
		FailoverVersionIncrement: 10,
		InitialFailoverVersion:   2,
		IsGlobalNamespaceEnabled: true,
		Tags:                     nil,
	}
}

func GetNamespaceReplicationMessagesResponse(resp *cadenceadmin.GetDomainReplicationMessagesResponse) *temporaladmin.GetNamespaceReplicationMessagesResponse {
	if resp == nil || resp.GetMessages() == nil {
		return nil
	}

	messages := &repication.ReplicationMessages{
		ReplicationTasks:       ReplicationTasks(resp.GetMessages().GetReplicationTasks()),
		LastRetrievedMessageId: resp.GetMessages().GetLastRetrievedMessageId(),
		HasMore:                resp.GetMessages().GetHasMore(),
		SyncShardStatus:        SyncShardStatus(resp.GetMessages().GetSyncShardStatus()),
	}

	return &temporaladmin.GetNamespaceReplicationMessagesResponse{
		Messages: messages,
	}
}

func GetWorkflowExecutionRawHistoryV2Response(
	resp *cadenceadmin.GetWorkflowExecutionRawHistoryV2Response,
) *temporaladmin.GetWorkflowExecutionRawHistoryV2Response {
	if resp == nil {
		return nil
	}

	historyBatches := make([]*temporal.DataBlob, len(resp.GetHistoryBatches()))
	for i, batch := range resp.GetHistoryBatches() {
		historyBatches[i] = Events(batch)
	}

	return &temporaladmin.GetWorkflowExecutionRawHistoryV2Response{
		NextPageToken:  resp.GetNextPageToken(),
		HistoryBatches: historyBatches,
		VersionHistory: VersionHistory(resp.GetVersionHistory()),
		//HistoryNodeIds: nil,
	}
}
