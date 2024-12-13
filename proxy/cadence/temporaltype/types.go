package temporaltype

import (
	"fmt"
	"github.com/gogo/protobuf/types"
	adminv1 "github.com/uber/cadence-idl/go/proto/admin/v1"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/command/v1"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/query/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/server/api/adminservice/v1"
	repication "go.temporal.io/server/api/replication/v1"
	servercommon "go.temporal.io/server/common"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/durationpb"
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

func TaskQueue(taskList *cadence.TaskList) *taskqueue.TaskQueue {
	if taskList == nil {
		return nil
	}

	return &taskqueue.TaskQueue{
		Name: taskList.GetName(),
		Kind: TaskQueueKind(taskList.GetKind()),
	}
}

func TaskQueueKind(kind cadence.TaskListKind) enums.TaskQueueKind {
	switch kind {
	case cadence.TaskListKind_TASK_LIST_KIND_NORMAL:
		return enums.TASK_QUEUE_KIND_NORMAL
	case cadence.TaskListKind_TASK_LIST_KIND_STICKY:
		return enums.TASK_QUEUE_KIND_STICKY
	case cadence.TaskListKind_TASK_LIST_KIND_INVALID:
		return enums.TASK_QUEUE_KIND_UNSPECIFIED
	default:
		return enums.TASK_QUEUE_KIND_UNSPECIFIED
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

func StickyAttributes(attributes *cadence.StickyExecutionAttributes) *taskqueue.StickyExecutionAttributes {
	return nil
}

func Commands(decisions []*cadence.Decision, wsClient workflowservice.WorkflowServiceClient, taskToken []byte) []*command.Command {
	if decisions == nil {
		return nil
	}

	commands := make([]*command.Command, len(decisions))
	for i, decision := range decisions {
		commands[i] = Command(decision, wsClient, taskToken)
	}
	return commands
}

func Command(
	decision *cadence.Decision,
	wsClient workflowservice.WorkflowServiceClient,
	taskToken []byte,
) *command.Command {
	if decision == nil {
		return nil
	}

	c := &command.Command{}

	switch decision.Attributes.(type) {
	case *cadence.Decision_ScheduleActivityTaskDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK
		c.Attributes = ScheduleActivityTaskCommandAttributes(decision.GetScheduleActivityTaskDecisionAttributes())
	case *cadence.Decision_StartTimerDecisionAttributes:
	case *cadence.Decision_CompleteWorkflowExecutionDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_COMPLETE_WORKFLOW_EXECUTION
		c.Attributes = CompleteWorkflowExecutionCommandAttributes(decision.GetCompleteWorkflowExecutionDecisionAttributes())
	case *cadence.Decision_FailWorkflowExecutionDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_FAIL_WORKFLOW_EXECUTION
		c.Attributes = FailWorkflowExecutionCommandAttributes(decision.GetFailWorkflowExecutionDecisionAttributes())
	case *cadence.Decision_RequestCancelActivityTaskDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_REQUEST_CANCEL_ACTIVITY_TASK
		c.Attributes = RequestCancelActivityTaskCommandAttributes(decision.GetRequestCancelActivityTaskDecisionAttributes(), wsClient, taskToken)
	case *cadence.Decision_CancelTimerDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_CancelWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_RequestCancelExternalWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_RecordMarkerDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_ContinueAsNewWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_StartChildWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_SignalExternalWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_UpsertWorkflowSearchAttributesDecisionAttributes:
		panic("not implemented" + decision.String())
	default:
		panic(fmt.Sprintf("not implemented decision type conversion: %T", decision.Attributes))
	}

	return c
}

func RequestCancelActivityTaskCommandAttributes(
	attributes *cadence.RequestCancelActivityTaskDecisionAttributes,
	wsClient workflowservice.WorkflowServiceClient,
	taskToken []byte,
) *command.Command_RequestCancelActivityTaskCommandAttributes {
	activityID := getScheduledEventIDByActivityID(attributes.GetActivityId(), wsClient, taskToken)

	return &command.Command_RequestCancelActivityTaskCommandAttributes{
		RequestCancelActivityTaskCommandAttributes: &command.RequestCancelActivityTaskCommandAttributes{
			ScheduledEventId: activityID,
		},
	}
}

func getScheduledEventIDByActivityID(activityID string, wsClient workflowservice.WorkflowServiceClient, token []byte) int64 {
	taskToken, err := servercommon.NewProtoTaskTokenSerializer().Deserialize(token)
	if err != nil {
		panic(fmt.Sprintf("failed to deserialize task token: %v", err))
	}

	descNSResq, err := wsClient.DescribeNamespace(context.Background(), &workflowservice.DescribeNamespaceRequest{
		Id: taskToken.GetNamespaceId(),
	})
	if err != nil {
		panic(fmt.Sprintf("failed to describe namespace: %v", err))
	}

	getHistoryResp, err := wsClient.GetWorkflowExecutionHistory(context.Background(), &workflowservice.GetWorkflowExecutionHistoryRequest{
		Namespace: descNSResq.GetNamespaceInfo().GetName(),
		Execution: &common.WorkflowExecution{
			WorkflowId: taskToken.GetWorkflowId(),
			RunId:      taskToken.GetRunId(),
		},
		MaximumPageSize: 0,
		NextPageToken:   nil,
		WaitNewEvent:    false,
		SkipArchival:    true,
	})

	if err != nil {
		panic(fmt.Sprintf("failed to get workflow history: %v", err))
	}

	events := getHistoryResp.GetHistory().GetEvents()
	for _, event := range events {
		if event.GetEventType() == enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED {
			attributes := event.GetActivityTaskScheduledEventAttributes()
			if attributes.GetActivityId() == activityID {
				return event.GetEventId()
			}
		}
	}

	panic(fmt.Sprintf("activity not found: %s", activityID))
}

func FailWorkflowExecutionCommandAttributes(
	attributes *cadence.FailWorkflowExecutionDecisionAttributes,
) *command.Command_FailWorkflowExecutionCommandAttributes {
	return &command.Command_FailWorkflowExecutionCommandAttributes{
		FailWorkflowExecutionCommandAttributes: &command.FailWorkflowExecutionCommandAttributes{
			Failure: Failure(attributes.GetFailure()),
		},
	}
}

func CompleteWorkflowExecutionCommandAttributes(
	attributes *cadence.CompleteWorkflowExecutionDecisionAttributes,
) *command.Command_CompleteWorkflowExecutionCommandAttributes {
	return &command.Command_CompleteWorkflowExecutionCommandAttributes{
		CompleteWorkflowExecutionCommandAttributes: &command.CompleteWorkflowExecutionCommandAttributes{
			Result: Payload(attributes.GetResult()),
		},
	}
}

func ScheduleActivityTaskCommandAttributes(
	attributes *cadence.ScheduleActivityTaskDecisionAttributes,
) *command.Command_ScheduleActivityTaskCommandAttributes {
	return &command.Command_ScheduleActivityTaskCommandAttributes{
		ScheduleActivityTaskCommandAttributes: &command.ScheduleActivityTaskCommandAttributes{
			ActivityId: attributes.GetActivityId(),
			ActivityType: &common.ActivityType{
				Name: attributes.GetActivityType().GetName(),
			},
			TaskQueue:              TaskQueue(attributes.GetTaskList()),
			Header:                 nil,
			Input:                  Payload(attributes.GetInput()),
			ScheduleToCloseTimeout: Duration(attributes.GetScheduleToCloseTimeout()),
			ScheduleToStartTimeout: Duration(attributes.GetScheduleToStartTimeout()),
			StartToCloseTimeout:    Duration(attributes.GetStartToCloseTimeout()),
			HeartbeatTimeout:       Duration(attributes.GetHeartbeatTimeout()),
			RetryPolicy:            RetryPolicy(attributes.GetRetryPolicy()),
			RequestEagerExecution:  false,
			UseWorkflowBuildId:     false,
		},
	}
}

func RetryPolicy(policy *cadence.RetryPolicy) *common.RetryPolicy {
	if policy == nil {
		return nil
	}

	return &common.RetryPolicy{
		InitialInterval:        Duration(policy.GetInitialInterval()),
		BackoffCoefficient:     policy.GetBackoffCoefficient(),
		MaximumInterval:        Duration(policy.GetMaximumInterval()),
		MaximumAttempts:        policy.GetMaximumAttempts(),
		NonRetryableErrorTypes: policy.GetNonRetryableErrorReasons(),
	}
}

func Payload(input *cadence.Payload) *common.Payloads {
	if input == nil {
		return nil
	}

	if payloads, err := converter.GetDefaultDataConverter().ToPayloads(input.GetData()); err != nil {
		panic(fmt.Sprintf("failed to convert cadence payload to temporal: %v", err))
	} else {
		return payloads
	}
}

func Duration(d *types.Duration) *durationpb.Duration {
	if d == nil {
		return nil
	}

	return &durationpb.Duration{
		Seconds: d.GetSeconds(),
		Nanos:   d.GetNanos(),
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

func Failure(f *cadence.Failure) *failure.Failure {
	if f == nil {
		return nil
	}

	return &failure.Failure{
		Message: f.GetReason(),
		//Source:            "",
		//StackTrace:        "",
		//EncodedAttributes: nil,
		//Cause:             nil,
		//FailureInfo:       nil,
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

func WorkflowExecution(execution *cadence.WorkflowExecution) *common.WorkflowExecution {
	if execution == nil {
		return nil
	}

	return &common.WorkflowExecution{
		WorkflowId: execution.GetWorkflowId(),
		RunId:      execution.GetRunId(),
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

func GetReplicationMessagesRequest(request *adminv1.GetReplicationMessagesRequest) *adminservice.GetReplicationMessagesRequest {
	if request == nil {
		return nil
	}

	return &adminservice.GetReplicationMessagesRequest{
		Tokens:      ReplicationToken(request.GetTokens()),
		ClusterName: request.GetClusterName(),
	}
}

func ReplicationToken(tokens []*adminv1.ReplicationToken) []*repication.ReplicationToken {
	if tokens == nil {
		return nil
	}

	result := make([]*repication.ReplicationToken, len(tokens))
	for i, token := range tokens {
		result[i] = &repication.ReplicationToken{
			ShardId:                token.GetShardId(),
			LastRetrievedMessageId: token.GetLastRetrievedMessageId(),
			LastProcessedMessageId: token.GetLastProcessedMessageId(),
			//LastProcessedVisibilityTime: nil,
		}
	}
	return result
}
