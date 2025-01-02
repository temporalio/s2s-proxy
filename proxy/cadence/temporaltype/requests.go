package temporaltype

import (
	cadenceadmin "github.com/uber/cadence-idl/go/proto/admin/v1"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/workflowservice/v1"
	temporaladmin "go.temporal.io/server/api/adminservice/v1"
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
