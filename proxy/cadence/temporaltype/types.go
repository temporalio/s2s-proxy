package temporaltype

import (
	cadenceadmin "github.com/uber/cadence-idl/go/proto/admin/v1"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/query/v1"
	"go.temporal.io/api/workflowservice/v1"
	temporaladmin "go.temporal.io/server/api/adminservice/v1"
	repication "go.temporal.io/server/api/replication/v1"
)

func PollWorkflowTaskQueueRequest(req *cadence.PollForDecisionTaskRequest) *workflowservice.PollWorkflowTaskQueueRequest {
	if req == nil {
		return nil
	}

	return &workflowservice.PollWorkflowTaskQueueRequest{
		Namespace:      req.GetDomain(),
		TaskQueue:      TaskQueue(req.GetTaskList()),
		Identity:       req.GetIdentity(),
		BinaryChecksum: req.GetBinaryChecksum(),
	}
}

func RespondWorkflowTaskCompletedRequest(
	request *cadence.RespondDecisionTaskCompletedRequest,
	wsClient workflowservice.WorkflowServiceClient,
) *workflowservice.RespondWorkflowTaskCompletedRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondWorkflowTaskCompletedRequest{
		TaskToken:                  request.GetTaskToken(),
		Commands:                   Commands(request.GetDecisions(), wsClient, request.GetTaskToken()),
		Identity:                   request.GetIdentity(),
		StickyAttributes:           StickyAttributes(request.GetStickyAttributes()),
		ReturnNewWorkflowTask:      request.GetReturnNewDecisionTask(),
		ForceCreateNewWorkflowTask: request.GetForceCreateNewDecisionTask(),
		BinaryChecksum:             request.GetBinaryChecksum(),
		QueryResults:               QueryResults(request.GetQueryResults()),
		//Namespace:                  "",
		//WorkerVersionStamp:         nil,
		//Messages:                   nil,
		//SdkMetadata:                nil,
		//MeteringMetadata:           nil,
	}
}

func QueryResults(results map[string]*cadence.WorkflowQueryResult) map[string]*query.WorkflowQueryResult {
	return nil
}

func PollActivityTaskQueueRequest(request *cadence.PollForActivityTaskRequest) *workflowservice.PollActivityTaskQueueRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.PollActivityTaskQueueRequest{
		Namespace: request.GetDomain(),
		TaskQueue: TaskQueue(request.GetTaskList()),
		Identity:  request.GetIdentity(),
		//TaskQueueMetadata:         nil,
		//WorkerVersionCapabilities: nil,
	}
}

func RespondActivityTaskCompletedRequest(request *cadence.RespondActivityTaskCompletedRequest) *workflowservice.RespondActivityTaskCompletedRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondActivityTaskCompletedRequest{
		TaskToken: request.GetTaskToken(),
		Result:    Payload(request.GetResult()),
		Identity:  request.GetIdentity(),
		//Namespace:     "",
		//WorkerVersion: nil,
	}

}

func RespondActivityTaskFailedRequest(request *cadence.RespondActivityTaskFailedRequest) *workflowservice.RespondActivityTaskFailedRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondActivityTaskFailedRequest{
		TaskToken: request.GetTaskToken(),
		Failure:   Failure(request.GetFailure()),
		Identity:  request.GetIdentity(),
		//Namespace:     "",
		//LastHeartbeatDetails:,
		//WorkerVersion: nil,
	}
}

func RecordActivityTaskHeartbeatRequest(
	request *cadence.RecordActivityTaskHeartbeatRequest,
) *workflowservice.RecordActivityTaskHeartbeatRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RecordActivityTaskHeartbeatRequest{
		TaskToken: request.GetTaskToken(),
		Details:   Payload(request.GetDetails()),
		Identity:  request.GetIdentity(),
		//Namespace: "",
	}
}

func ResetStickyTaskQueueRequest(
	request *cadence.ResetStickyTaskListRequest,
) *workflowservice.ResetStickyTaskQueueRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.ResetStickyTaskQueueRequest{
		Namespace: request.GetDomain(),
		Execution: WorkflowExecution(request.GetWorkflowExecution()),
	}
}

func RespondActivityTaskCanceledRequest(request *cadence.RespondActivityTaskCanceledRequest) *workflowservice.RespondActivityTaskCanceledRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondActivityTaskCanceledRequest{
		TaskToken: request.GetTaskToken(),
		Details:   Payload(request.GetDetails()),
		Identity:  request.GetIdentity(),
		//Namespace:     "",
		//WorkerVersion: nil,
	}
}

func GetReplicationMessagesRequest(request *cadenceadmin.GetReplicationMessagesRequest) *temporaladmin.GetReplicationMessagesRequest {
	if request == nil {
		return nil
	}

	return &temporaladmin.GetReplicationMessagesRequest{
		Tokens:      ReplicationToken(request.GetTokens()),
		ClusterName: request.GetClusterName(),
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
		HistoryShardCount:        4,
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

func GetNamespaceReplicationMessagesRequest(request *cadenceadmin.GetDomainReplicationMessagesRequest) *temporaladmin.GetNamespaceReplicationMessagesRequest {
	if request == nil {
		return nil
	}

	return &temporaladmin.GetNamespaceReplicationMessagesRequest{
		ClusterName:            request.GetClusterName(),
		LastRetrievedMessageId: Int64Value(request.GetLastRetrievedMessageId()),
		LastProcessedMessageId: Int64Value(request.GetLastProcessedMessageId()),
	}

}
