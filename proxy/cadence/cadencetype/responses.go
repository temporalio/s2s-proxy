package cadencetype

import (
	cadenceadmin "github.com/uber/cadence-idl/go/proto/admin/v1"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/workflowservice/v1"
	temporaladmin "go.temporal.io/server/api/adminservice/v1"
)

func PollForDecisionTaskResponse(
	resp *workflowservice.PollWorkflowTaskQueueResponse,
	wsClient workflowservice.WorkflowServiceClient,
) *cadence.PollForDecisionTaskResponse {
	if resp == nil {
		return nil
	}

	return &cadence.PollForDecisionTaskResponse{
		TaskToken:                 resp.GetTaskToken(),
		WorkflowExecution:         WorkflowExecution(resp.GetWorkflowExecution()),
		WorkflowType:              WorkflowType(resp.GetWorkflowType()),
		PreviousStartedEventId:    Int64ValuePtr(resp.GetPreviousStartedEventId()),
		StartedEventId:            resp.GetStartedEventId(),
		Attempt:                   int64(resp.GetAttempt()),
		BacklogCountHint:          resp.GetBacklogCountHint(),
		History:                   History(resp.GetHistory(), wsClient, resp.GetTaskToken()),
		NextPageToken:             resp.GetNextPageToken(),
		WorkflowExecutionTaskList: TaskList(resp.GetWorkflowExecutionTaskQueue()),
		ScheduledTime:             Timestamp(resp.GetScheduledTime()),
		StartedTime:               Timestamp(resp.GetStartedTime()),
	}
}

func RespondDecisionTaskCompletedResponse(
	resp *workflowservice.RespondWorkflowTaskCompletedResponse,
) *cadence.RespondDecisionTaskCompletedResponse {
	if resp == nil {
		return nil
	}

	return &cadence.RespondDecisionTaskCompletedResponse{}
}

func PollForActivityTaskResponse(resp *workflowservice.PollActivityTaskQueueResponse) *cadence.PollForActivityTaskResponse {
	if resp == nil {
		return nil
	}

	return &cadence.PollForActivityTaskResponse{
		TaskToken:                  resp.GetTaskToken(),
		WorkflowExecution:          WorkflowExecution(resp.GetWorkflowExecution()),
		ActivityId:                 resp.GetActivityId(),
		ActivityType:               ActivityType(resp.GetActivityType()),
		Input:                      Payloads(resp.GetInput()),
		ScheduledTime:              Timestamp(resp.GetScheduledTime()),
		StartedTime:                Timestamp(resp.GetStartedTime()),
		ScheduleToCloseTimeout:     Duration(resp.GetScheduleToCloseTimeout()),
		StartToCloseTimeout:        Duration(resp.GetStartToCloseTimeout()),
		HeartbeatTimeout:           Duration(resp.GetHeartbeatTimeout()),
		Attempt:                    resp.GetAttempt(),
		ScheduledTimeOfThisAttempt: Timestamp(resp.GetCurrentAttemptScheduledTime()),
		HeartbeatDetails:           Payloads(resp.GetHeartbeatDetails()),
		WorkflowType:               WorkflowType(resp.GetWorkflowType()),
		WorkflowDomain:             resp.GetWorkflowNamespace(),
		Header:                     Header(resp.GetHeader()),
		//AutoConfigHint:             nil,
	}
}

func RespondActivityTaskCompletedResponse(resp *workflowservice.RespondActivityTaskCompletedResponse) *cadence.RespondActivityTaskCompletedResponse {
	if resp == nil {
		return nil
	}

	return &cadence.RespondActivityTaskCompletedResponse{}
}

func RespondActivityTaskFailedResponse(
	resp *workflowservice.RespondActivityTaskFailedResponse,
) *cadence.RespondActivityTaskFailedResponse {
	if resp == nil {
		return nil
	}

	return &cadence.RespondActivityTaskFailedResponse{}
}

func RecordActivityTaskHeartbeatResponse(
	resp *workflowservice.RecordActivityTaskHeartbeatResponse,
) *cadence.RecordActivityTaskHeartbeatResponse {
	if resp == nil {
		return nil
	}

	return &cadence.RecordActivityTaskHeartbeatResponse{
		CancelRequested: resp.GetCancelRequested(),
	}
}

func ResetStickyTaskListResponse(resp *workflowservice.ResetStickyTaskQueueResponse) *cadence.ResetStickyTaskListResponse {
	if resp == nil {
		return nil
	}

	return &cadence.ResetStickyTaskListResponse{}
}

func RespondActivityTaskCanceledResponse(resp *workflowservice.RespondActivityTaskCanceledResponse) *cadence.RespondActivityTaskCanceledResponse {
	if resp == nil {
		return nil
	}

	return &cadence.RespondActivityTaskCanceledResponse{}
}

func GetReplicationMessagesResponse(resp *temporaladmin.GetReplicationMessagesResponse) *cadenceadmin.GetReplicationMessagesResponse {
	if resp == nil {
		return nil
	}

	return &cadenceadmin.GetReplicationMessagesResponse{
		ShardMessages: ShardMessages(resp.GetShardMessages()),
	}
}

func GetDomainReplicationMessagesResponse(resp *temporaladmin.GetNamespaceReplicationMessagesResponse) *cadenceadmin.GetDomainReplicationMessagesResponse {
	if resp == nil {
		return nil
	}

	return &cadenceadmin.GetDomainReplicationMessagesResponse{
		Messages: ReplicationMessages(resp.GetMessages()),
	}
}
