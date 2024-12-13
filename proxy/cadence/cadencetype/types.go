package cadencetype

import (
	"context"
	"fmt"
	"github.com/gogo/protobuf/types"
	adminv1 "github.com/uber/cadence-idl/go/proto/admin/v1"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	temporal "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/replication/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	repication "go.temporal.io/server/api/replication/v1"
	servercommon "go.temporal.io/server/common"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func WorkflowExecution(e *temporal.WorkflowExecution) *cadence.WorkflowExecution {
	return &cadence.WorkflowExecution{
		WorkflowId: e.GetWorkflowId(),
		RunId:      e.GetRunId(),
	}
}

func WorkflowType(t *temporal.WorkflowType) *cadence.WorkflowType {
	return &cadence.WorkflowType{
		Name: t.GetName(),
	}
}

func Int64ValuePtr(i int64) *types.Int64Value {
	return &types.Int64Value{
		Value: i,
	}
}

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

func History(h *history.History, wsClient workflowservice.WorkflowServiceClient, taskToken []byte) *cadence.History {
	if h == nil {
		return nil
	}
	events := make([]*cadence.HistoryEvent, 0, len(h.GetEvents()))
	for _, e := range h.Events {
		events = append(events, HistoryEvent(e, wsClient, taskToken))
	}
	return &cadence.History{Events: events}
}

func HistoryEvent(e *history.HistoryEvent, wsClient workflowservice.WorkflowServiceClient, taskToken []byte) *cadence.HistoryEvent {
	event := &cadence.HistoryEvent{
		EventId:   e.GetEventId(),
		EventTime: Timestamp(e.GetEventTime()),
		Version:   e.GetVersion(),
		TaskId:    e.GetTaskId(),
	}

	switch e.Attributes.(type) {
	case *history.HistoryEvent_WorkflowExecutionStartedEventAttributes:
		event.Attributes = WorkflowExecutionStartedEventAttributes(e.GetWorkflowExecutionStartedEventAttributes())
	case *history.HistoryEvent_WorkflowTaskScheduledEventAttributes:
		event.Attributes = WorkflowTaskScheduledEventAttributes(e.GetWorkflowTaskScheduledEventAttributes())
	case *history.HistoryEvent_WorkflowTaskStartedEventAttributes:
		event.Attributes = WorkflowTaskStartedEventAttributes(e.GetWorkflowTaskStartedEventAttributes())
	case *history.HistoryEvent_WorkflowTaskTimedOutEventAttributes:
		event.Attributes = WorkflowTaskTimedOutEventAttributes(e.GetWorkflowTaskTimedOutEventAttributes())
	case *history.HistoryEvent_WorkflowTaskFailedEventAttributes:
		event.Attributes = WorkflowTaskFailedEventAttributes(e.GetWorkflowTaskFailedEventAttributes())
	case *history.HistoryEvent_WorkflowTaskCompletedEventAttributes:
		event.Attributes = WorkflowTaskCompletedEventAttributes(e.GetWorkflowTaskCompletedEventAttributes())
	case *history.HistoryEvent_ActivityTaskScheduledEventAttributes:
		event.Attributes = ActivityTaskScheduledEventAttributes(e.GetActivityTaskScheduledEventAttributes())
	case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
		event.Attributes = ActivityTaskStartedEventAttributes(e.GetActivityTaskStartedEventAttributes())
	case *history.HistoryEvent_ActivityTaskCompletedEventAttributes:
		event.Attributes = ActivityTaskCompletedEventAttributes(e.GetActivityTaskCompletedEventAttributes())
	case *history.HistoryEvent_ActivityTaskFailedEventAttributes:
		event.Attributes = ActivityTaskFailedEventAttributes(e.GetActivityTaskFailedEventAttributes())
	case *history.HistoryEvent_ActivityTaskTimedOutEventAttributes:
		event.Attributes = ActivityTaskTimedOutEventAttributes(e.GetActivityTaskTimedOutEventAttributes())
	case *history.HistoryEvent_ActivityTaskCancelRequestedEventAttributes:
		event.Attributes = ActivityTaskCancelRequestedEventAttributes(e.GetActivityTaskCancelRequestedEventAttributes(), wsClient, taskToken)
	case *history.HistoryEvent_WorkflowExecutionSignaledEventAttributes:
		event.Attributes = WorkflowExecutionSignaledEventAttributes(e.GetWorkflowExecutionSignaledEventAttributes())
	case *history.HistoryEvent_WorkflowExecutionCancelRequestedEventAttributes:
		event.Attributes = WorkflowExecutionCancelRequestedEventAttributes(e.GetWorkflowExecutionCancelRequestedEventAttributes())
	case *history.HistoryEvent_ActivityTaskCanceledEventAttributes:
		event.Attributes = ActivityTaskCanceledEventAttributes(e.GetActivityTaskCanceledEventAttributes())
	default:
		fmt.Printf("Liang: event type not converted %T to cadence type\n", e.Attributes)
	}

	return event
}

func ActivityTaskCanceledEventAttributes(
	attributes *history.ActivityTaskCanceledEventAttributes,
) *cadence.HistoryEvent_ActivityTaskCanceledEventAttributes {
	return &cadence.HistoryEvent_ActivityTaskCanceledEventAttributes{
		ActivityTaskCanceledEventAttributes: &cadence.ActivityTaskCanceledEventAttributes{
			Details:                      Payloads(attributes.GetDetails()),
			LatestCancelRequestedEventId: attributes.GetLatestCancelRequestedEventId(),
			ScheduledEventId:             attributes.GetScheduledEventId(),
			StartedEventId:               attributes.GetStartedEventId(),
			Identity:                     attributes.GetIdentity(),
		},
	}
}

func WorkflowExecutionCancelRequestedEventAttributes(
	attributes *history.WorkflowExecutionCancelRequestedEventAttributes,
) *cadence.HistoryEvent_WorkflowExecutionCancelRequestedEventAttributes {
	return &cadence.HistoryEvent_WorkflowExecutionCancelRequestedEventAttributes{
		WorkflowExecutionCancelRequestedEventAttributes: &cadence.WorkflowExecutionCancelRequestedEventAttributes{
			Cause:    attributes.GetCause(),
			Identity: attributes.GetIdentity(),
			ExternalExecutionInfo: &cadence.ExternalExecutionInfo{
				InitiatedId:       attributes.GetExternalInitiatedEventId(),
				WorkflowExecution: WorkflowExecution(attributes.GetExternalWorkflowExecution()),
			},
			//RequestId:             "",
		},
	}
}

func WorkflowExecutionSignaledEventAttributes(
	attributes *history.WorkflowExecutionSignaledEventAttributes,
) *cadence.HistoryEvent_WorkflowExecutionSignaledEventAttributes {
	return &cadence.HistoryEvent_WorkflowExecutionSignaledEventAttributes{
		WorkflowExecutionSignaledEventAttributes: &cadence.WorkflowExecutionSignaledEventAttributes{
			SignalName: attributes.GetSignalName(),
			Input:      Payloads(attributes.GetInput()),
			Identity:   attributes.GetIdentity(),
		},
	}
}

func ActivityTaskCancelRequestedEventAttributes(
	attributes *history.ActivityTaskCancelRequestedEventAttributes,
	wsClient workflowservice.WorkflowServiceClient,
	token []byte,
) *cadence.HistoryEvent_ActivityTaskCancelRequestedEventAttributes {
	activityID := getActivityIDFromScheduledEventID(attributes, wsClient, token)

	return &cadence.HistoryEvent_ActivityTaskCancelRequestedEventAttributes{
		ActivityTaskCancelRequestedEventAttributes: &cadence.ActivityTaskCancelRequestedEventAttributes{
			ActivityId:                   activityID,
			DecisionTaskCompletedEventId: attributes.GetWorkflowTaskCompletedEventId(),
		},
	}
}

func getActivityIDFromScheduledEventID(attributes *history.ActivityTaskCancelRequestedEventAttributes, wsClient workflowservice.WorkflowServiceClient, token []byte) string {
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
		Execution: &temporal.WorkflowExecution{
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

	for _, e := range getHistoryResp.GetHistory().GetEvents() {
		if e.GetEventId() == attributes.GetScheduledEventId() {
			if e.GetEventType() != enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED {
				panic(fmt.Sprintf("scheduled event is not activity task scheduled: %v", e))
			}
			return e.GetActivityTaskScheduledEventAttributes().GetActivityId()
		}
	}

	panic(fmt.Sprintf("scheduled event not found: %v", attributes.GetScheduledEventId()))
}

func ActivityTaskTimedOutEventAttributes(attributes *history.ActivityTaskTimedOutEventAttributes) *cadence.HistoryEvent_ActivityTaskTimedOutEventAttributes {
	return &cadence.HistoryEvent_ActivityTaskTimedOutEventAttributes{
		ActivityTaskTimedOutEventAttributes: &cadence.ActivityTaskTimedOutEventAttributes{
			//Details:          Payloads(attributes.GetRetryState()),
			ScheduledEventId: attributes.GetScheduledEventId(),
			StartedEventId:   attributes.GetStartedEventId(),
			//TimeoutType:      TimeoutType(attributes.GetTimeoutType()),
			LastFailure: Failure(attributes.GetFailure()),
		},
	}
}

func ActivityTaskFailedEventAttributes(attributes *history.ActivityTaskFailedEventAttributes) *cadence.HistoryEvent_ActivityTaskFailedEventAttributes {
	return &cadence.HistoryEvent_ActivityTaskFailedEventAttributes{
		ActivityTaskFailedEventAttributes: &cadence.ActivityTaskFailedEventAttributes{
			Failure:          Failure(attributes.GetFailure()),
			ScheduledEventId: attributes.GetScheduledEventId(),
			StartedEventId:   attributes.GetStartedEventId(),
			Identity:         attributes.GetIdentity(),
		},
	}
}

func ActivityTaskCompletedEventAttributes(attributes *history.ActivityTaskCompletedEventAttributes) *cadence.HistoryEvent_ActivityTaskCompletedEventAttributes {
	return &cadence.HistoryEvent_ActivityTaskCompletedEventAttributes{
		ActivityTaskCompletedEventAttributes: &cadence.ActivityTaskCompletedEventAttributes{
			Result:           Payloads(attributes.GetResult()),
			ScheduledEventId: attributes.GetScheduledEventId(),
			StartedEventId:   attributes.GetStartedEventId(),
			Identity:         attributes.GetIdentity(),
		},
	}
}

func ActivityTaskStartedEventAttributes(attributes *history.ActivityTaskStartedEventAttributes) *cadence.HistoryEvent_ActivityTaskStartedEventAttributes {
	return &cadence.HistoryEvent_ActivityTaskStartedEventAttributes{
		ActivityTaskStartedEventAttributes: &cadence.ActivityTaskStartedEventAttributes{
			ScheduledEventId: attributes.GetScheduledEventId(),
			Identity:         attributes.GetIdentity(),
			RequestId:        attributes.GetRequestId(),
			Attempt:          attributes.GetAttempt(),
			LastFailure:      Failure(attributes.GetLastFailure()),
		},
	}
}

func ActivityTaskScheduledEventAttributes(
	attributes *history.ActivityTaskScheduledEventAttributes,
) *cadence.HistoryEvent_ActivityTaskScheduledEventAttributes {
	return &cadence.HistoryEvent_ActivityTaskScheduledEventAttributes{
		ActivityTaskScheduledEventAttributes: &cadence.ActivityTaskScheduledEventAttributes{
			ActivityId:   attributes.GetActivityId(),
			ActivityType: ActivityType(attributes.GetActivityType()),
			//Domain:                     "",
			TaskList:                     TaskList(attributes.GetTaskQueue()),
			Input:                        Payloads(attributes.GetInput()),
			ScheduleToCloseTimeout:       Duration(attributes.GetScheduleToCloseTimeout()),
			ScheduleToStartTimeout:       Duration(attributes.GetScheduleToStartTimeout()),
			StartToCloseTimeout:          Duration(attributes.GetStartToCloseTimeout()),
			HeartbeatTimeout:             Duration(attributes.GetHeartbeatTimeout()),
			DecisionTaskCompletedEventId: attributes.GetWorkflowTaskCompletedEventId(),
			RetryPolicy:                  RetryPolicy(attributes.GetRetryPolicy()),
			Header:                       Header(attributes.GetHeader()),
		},
	}
}

func ActivityType(activityType *temporal.ActivityType) *cadence.ActivityType {
	if activityType == nil {
		return nil
	}

	return &cadence.ActivityType{
		Name: activityType.GetName(),
	}
}

func WorkflowTaskCompletedEventAttributes(
	attributes *history.WorkflowTaskCompletedEventAttributes,
) *cadence.HistoryEvent_DecisionTaskCompletedEventAttributes {
	return &cadence.HistoryEvent_DecisionTaskCompletedEventAttributes{
		DecisionTaskCompletedEventAttributes: &cadence.DecisionTaskCompletedEventAttributes{
			ScheduledEventId: attributes.GetScheduledEventId(),
			StartedEventId:   attributes.GetStartedEventId(),
			Identity:         attributes.GetIdentity(),
			BinaryChecksum:   attributes.GetBinaryChecksum(),
			//ExecutionContext: nil,
		},
	}
}

func WorkflowTaskFailedEventAttributes(
	attributes *history.WorkflowTaskFailedEventAttributes,
) *cadence.HistoryEvent_DecisionTaskFailedEventAttributes {
	return &cadence.HistoryEvent_DecisionTaskFailedEventAttributes{
		DecisionTaskFailedEventAttributes: &cadence.DecisionTaskFailedEventAttributes{
			ScheduledEventId: attributes.GetScheduledEventId(),
			StartedEventId:   attributes.GetStartedEventId(),
			Cause:            DecisionTaskFailedCause(attributes.GetCause()),
			Failure:          Failure(attributes.GetFailure()),
			Identity:         attributes.GetIdentity(),
			BaseRunId:        attributes.GetBaseRunId(),
			NewRunId:         attributes.GetNewRunId(),
			ForkEventVersion: attributes.GetForkEventVersion(),
			BinaryChecksum:   attributes.GetBinaryChecksum(),
			//RequestId:        attributes.GetRequestId(),
		},
	}

}

func DecisionTaskFailedCause(cause enums.WorkflowTaskFailedCause) cadence.DecisionTaskFailedCause {
	switch cause {
	case enums.WORKFLOW_TASK_FAILED_CAUSE_UNHANDLED_COMMAND:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_UNHANDLED_DECISION
	case enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_SCHEDULE_ACTIVITY_ATTRIBUTES:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_BAD_SCHEDULE_ACTIVITY_ATTRIBUTES
	case enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_REQUEST_CANCEL_ACTIVITY_ATTRIBUTES:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_BAD_REQUEST_CANCEL_ACTIVITY_ATTRIBUTES
	case enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_START_TIMER_ATTRIBUTES:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_BAD_START_TIMER_ATTRIBUTES
	case enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_CANCEL_TIMER_ATTRIBUTES:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_BAD_CANCEL_TIMER_ATTRIBUTES
	case enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_RECORD_MARKER_ATTRIBUTES:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_BAD_RECORD_MARKER_ATTRIBUTES
	case enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_CONTINUE_AS_NEW_ATTRIBUTES:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_BAD_CONTINUE_AS_NEW_ATTRIBUTES
	case enums.WORKFLOW_TASK_FAILED_CAUSE_RESET_STICKY_TASK_QUEUE:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_RESET_STICKY_TASK_LIST
	case enums.WORKFLOW_TASK_FAILED_CAUSE_WORKFLOW_WORKER_UNHANDLED_FAILURE:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_WORKFLOW_WORKER_UNHANDLED_FAILURE
	case enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_SIGNAL_INPUT_SIZE:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_BAD_SIGNAL_INPUT_SIZE
	case enums.WORKFLOW_TASK_FAILED_CAUSE_RESET_WORKFLOW:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_RESET_WORKFLOW
	case enums.WORKFLOW_TASK_FAILED_CAUSE_UNSPECIFIED:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_INVALID
	default:
		return cadence.DecisionTaskFailedCause_DECISION_TASK_FAILED_CAUSE_INVALID
	}
}

func WorkflowTaskTimedOutEventAttributes(
	attributes *history.WorkflowTaskTimedOutEventAttributes,
) *cadence.HistoryEvent_DecisionTaskTimedOutEventAttributes {
	return &cadence.HistoryEvent_DecisionTaskTimedOutEventAttributes{
		DecisionTaskTimedOutEventAttributes: &cadence.DecisionTaskTimedOutEventAttributes{
			ScheduledEventId: attributes.GetScheduledEventId(),
			StartedEventId:   attributes.GetStartedEventId(),
			TimeoutType:      TimeoutType(attributes.GetTimeoutType()),
			//BaseRunId:        "",
			//NewRunId:         "",
			//ForkEventVersion: 0,
			//Reason:           "",
			//Cause:            0,
			//RequestId:        "",
		},
	}

}

func TimeoutType(timeoutType enums.TimeoutType) cadence.TimeoutType {
	return cadence.TimeoutType(timeoutType)
}

func WorkflowTaskStartedEventAttributes(
	attributes *history.WorkflowTaskStartedEventAttributes,
) *cadence.HistoryEvent_DecisionTaskStartedEventAttributes {
	return &cadence.HistoryEvent_DecisionTaskStartedEventAttributes{
		DecisionTaskStartedEventAttributes: &cadence.DecisionTaskStartedEventAttributes{
			ScheduledEventId: attributes.GetScheduledEventId(),
			Identity:         attributes.GetIdentity(),
			RequestId:        attributes.GetRequestId(),
		},
	}
}

func WorkflowTaskScheduledEventAttributes(
	attributes *history.WorkflowTaskScheduledEventAttributes,
) *cadence.HistoryEvent_DecisionTaskScheduledEventAttributes {
	return &cadence.HistoryEvent_DecisionTaskScheduledEventAttributes{
		DecisionTaskScheduledEventAttributes: &cadence.DecisionTaskScheduledEventAttributes{
			TaskList:            TaskList(attributes.GetTaskQueue()),
			StartToCloseTimeout: Duration(attributes.GetStartToCloseTimeout()),
			Attempt:             attributes.GetAttempt(),
		},
	}
}

func WorkflowExecutionStartedEventAttributes(
	attributes *history.WorkflowExecutionStartedEventAttributes,
) *cadence.HistoryEvent_WorkflowExecutionStartedEventAttributes {
	return &cadence.HistoryEvent_WorkflowExecutionStartedEventAttributes{
		WorkflowExecutionStartedEventAttributes: &cadence.WorkflowExecutionStartedEventAttributes{
			WorkflowType:                 WorkflowType(attributes.GetWorkflowType()),
			ParentExecutionInfo:          ParentExecutionInfo(attributes.GetParentWorkflowExecution()),
			TaskList:                     TaskList(attributes.GetTaskQueue()),
			Input:                        Payloads(attributes.GetInput()),
			ExecutionStartToCloseTimeout: Duration(attributes.GetWorkflowExecutionTimeout()),
			//TaskStartToCloseTimeout:      nil,
			ContinuedExecutionRunId:  attributes.GetContinuedExecutionRunId(),
			Initiator:                Initiator(attributes.GetInitiator()),
			ContinuedFailure:         Failure(attributes.GetContinuedFailure()),
			LastCompletionResult:     Payloads(attributes.GetLastCompletionResult()),
			OriginalExecutionRunId:   attributes.GetOriginalExecutionRunId(),
			Identity:                 attributes.GetIdentity(),
			FirstExecutionRunId:      attributes.GetFirstExecutionRunId(),
			RetryPolicy:              RetryPolicy(attributes.GetRetryPolicy()),
			Attempt:                  attributes.GetAttempt(),
			ExpirationTime:           Timestamp(attributes.GetWorkflowExecutionExpirationTime()),
			CronSchedule:             attributes.GetCronSchedule(),
			FirstDecisionTaskBackoff: Duration(attributes.GetFirstWorkflowTaskBackoff()),
			Memo:                     Memo(attributes.GetMemo()),
			SearchAttributes:         SearchAttributes(attributes.GetSearchAttributes()),
			PrevAutoResetPoints:      ReseetPoints(attributes.GetPrevAutoResetPoints()),
			Header:                   Header(attributes.GetHeader()),
			FirstScheduledTime:       nil,
			//PartitionConfig:              nil,
			//RequestId:                    "",
		},
	}
}

func ParentExecutionInfo(parentExecution *temporal.WorkflowExecution) *cadence.ParentExecutionInfo {
	if parentExecution == nil {
		return nil
	}

	return &cadence.ParentExecutionInfo{
		//DomainId:          "",
		//DomainName:        "",
		WorkflowExecution: &cadence.WorkflowExecution{
			WorkflowId: parentExecution.GetWorkflowId(),
			RunId:      parentExecution.GetRunId(),
		},
		//InitiatedId:       0,
	}
}

func Initiator(initiator enums.ContinueAsNewInitiator) cadence.ContinueAsNewInitiator {
	switch initiator {
	case enums.CONTINUE_AS_NEW_INITIATOR_WORKFLOW:
		return cadence.ContinueAsNewInitiator_CONTINUE_AS_NEW_INITIATOR_DECIDER
	case enums.CONTINUE_AS_NEW_INITIATOR_RETRY:
		return cadence.ContinueAsNewInitiator_CONTINUE_AS_NEW_INITIATOR_RETRY_POLICY
	case enums.CONTINUE_AS_NEW_INITIATOR_CRON_SCHEDULE:
		return cadence.ContinueAsNewInitiator_CONTINUE_AS_NEW_INITIATOR_CRON_SCHEDULE
	default:
		return cadence.ContinueAsNewInitiator_CONTINUE_AS_NEW_INITIATOR_INVALID
	}
}

func ReseetPoints(points *workflow.ResetPoints) *cadence.ResetPoints {
	return nil
}

func Header(header *temporal.Header) *cadence.Header {
	return nil
}

func SearchAttributes(attributes *temporal.SearchAttributes) *cadence.SearchAttributes {
	return nil
}

func Memo(memo *temporal.Memo) *cadence.Memo {
	return nil
}

func RetryPolicy(policy *temporal.RetryPolicy) *cadence.RetryPolicy {
	if policy == nil {
		return nil
	}

	return &cadence.RetryPolicy{
		InitialInterval:          Duration(policy.GetInitialInterval()),
		BackoffCoefficient:       policy.GetBackoffCoefficient(),
		MaximumInterval:          Duration(policy.GetMaximumInterval()),
		MaximumAttempts:          policy.GetMaximumAttempts(),
		NonRetryableErrorReasons: policy.GetNonRetryableErrorTypes(),
		//ExpirationInterval:       nil,
	}
}

func Payloads(payloads *temporal.Payloads) *cadence.Payload {
	if payloads == nil || len(payloads.GetPayloads()) == 0 {
		return nil
	}

	if len(payloads.GetPayloads()) > 1 {
		panic(fmt.Sprintf("more than one payload is not supported: %v", payloads.GetPayloads()))
	}

	return Payload(payloads.GetPayloads()[0])
}

func Payload(payload *temporal.Payload) *cadence.Payload {
	if payload == nil {
		return nil
	}

	return &cadence.Payload{
		Data: payload.GetData(),
	}
}

func Failure(failure *failure.Failure) *cadence.Failure {
	if failure == nil {
		return nil
	}

	return &cadence.Failure{
		Reason: failure.GetMessage(),
	}
}

func Duration(d *durationpb.Duration) *types.Duration {
	if d == nil {
		return nil
	}

	return &types.Duration{
		Seconds: d.GetSeconds(),
		Nanos:   d.GetNanos(),
	}
}

func TaskList(tq *taskqueue.TaskQueue) *cadence.TaskList {
	if tq == nil {
		return nil
	}

	return &cadence.TaskList{
		Name: tq.GetName(),
		Kind: TaskListKind(tq.GetKind()),
	}
}

func TaskListKind(kind enums.TaskQueueKind) cadence.TaskListKind {
	switch kind {
	case enums.TASK_QUEUE_KIND_NORMAL:
		return cadence.TaskListKind_TASK_LIST_KIND_NORMAL
	case enums.TASK_QUEUE_KIND_STICKY:
		return cadence.TaskListKind_TASK_LIST_KIND_STICKY
	case enums.TASK_QUEUE_KIND_UNSPECIFIED:
		return cadence.TaskListKind_TASK_LIST_KIND_INVALID
	default:
		return cadence.TaskListKind_TASK_LIST_KIND_INVALID
	}
}

func Timestamp(t *timestamppb.Timestamp) *types.Timestamp {
	if t == nil {
		return nil
	}

	return &types.Timestamp{
		Seconds: t.GetSeconds(),
		Nanos:   t.GetNanos(),
	}
}

func Error(err error) error {
	return err
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

func GetReplicationMessagesResponse(resp *adminservice.GetReplicationMessagesResponse) *adminv1.GetReplicationMessagesResponse {
	if resp == nil {
		return nil
	}

	return &adminv1.GetReplicationMessagesResponse{
		ShardMessages: ShardMessages(resp.GetShardMessages()),
	}
}

func ShardMessages(messages map[int32]*repication.ReplicationMessages) map[int32]*adminv1.ReplicationMessages {
	if messages == nil {
		return nil
	}

	result := make(map[int32]*adminv1.ReplicationMessages, len(messages))
	for k, v := range messages {
		result[k] = ReplicationMessages(v)
	}
	return result
}

func ReplicationMessages(v *repication.ReplicationMessages) *adminv1.ReplicationMessages {
	if v == nil {
		return nil
	}

	return &adminv1.ReplicationMessages{
		ReplicationTasks:       ReplicationTasks(v.GetReplicationTasks()),
		LastRetrievedMessageId: v.GetLastRetrievedMessageId(),
		HasMore:                v.GetHasMore(),
		SyncShardStatus:        SyncShardStatus(v.GetSyncShardStatus()),
	}
}

func SyncShardStatus(status *repication.SyncShardStatus) *adminv1.SyncShardStatus {
	if status == nil {
		return nil
	}

	return &adminv1.SyncShardStatus{
		Timestamp: Timestamp(status.GetStatusTime()),
	}
}

func ReplicationTasks(tasks []*repication.ReplicationTask) []*adminv1.ReplicationTask {
	if tasks == nil {
		return nil
	}

	result := make([]*adminv1.ReplicationTask, 0, len(tasks))
	for _, t := range tasks {
		result = append(result, ReplicationTask(t))
	}
	return result
}

func ReplicationTask(t *repication.ReplicationTask) *adminv1.ReplicationTask {
	if t == nil {
		return nil
	}

	replicationTask := &adminv1.ReplicationTask{
		SourceTaskId: t.GetSourceTaskId(),
		CreationTime: Timestamp(t.GetVisibilityTime()),
	}

	switch t.GetTaskType() {
	case enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK:
		replicationTask.TaskType = adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_DOMAIN
		replicationTask.Attributes = DomainTaskAttributes(t.GetNamespaceTaskAttributes())
	case enumsspb.REPLICATION_TASK_TYPE_HISTORY_TASK:
		replicationTask.TaskType = adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_HISTORY
	case enumsspb.REPLICATION_TASK_TYPE_SYNC_SHARD_STATUS_TASK:
		replicationTask.TaskType = adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_SYNC_SHARD_STATUS
	case enumsspb.REPLICATION_TASK_TYPE_SYNC_ACTIVITY_TASK:
		replicationTask.TaskType = adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_SYNC_ACTIVITY
	case enumsspb.REPLICATION_TASK_TYPE_HISTORY_METADATA_TASK:
		replicationTask.TaskType = adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_HISTORY_METADATA
	case enumsspb.REPLICATION_TASK_TYPE_HISTORY_V2_TASK:
		replicationTask.TaskType = adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_HISTORY_V2
	default:
		panic(fmt.Sprintf("unsupported replication task type: %v", t.GetTaskType()))
	}

	return replicationTask
}

func DomainTaskAttributes(attributes *repication.NamespaceTaskAttributes) *adminv1.ReplicationTask_DomainTaskAttributes {
	if attributes == nil {
		return nil
	}

	return &adminv1.ReplicationTask_DomainTaskAttributes{
		DomainTaskAttributes: &adminv1.DomainTaskAttributes{
			DomainOperation: DomainOperation(attributes.GetNamespaceOperation()),
			Id:              attributes.GetId(),
			Domain:          Domain(attributes.GetInfo(), attributes.GetConfig(), attributes.GetReplicationConfig(), attributes.GetFailoverVersion()),
			ConfigVersion:   attributes.GetConfigVersion(),
			FailoverVersion: attributes.GetFailoverVersion(),
			//PreviousFailoverVersion: 0,
		},
	}
}

func Domain(
	info *namespace.NamespaceInfo,
	config *namespace.NamespaceConfig,
	replicationConfig *replication.NamespaceReplicationConfig,
	failoverVersion int64,
) *cadence.Domain {
	if info == nil {
		return nil
	}

	return &cadence.Domain{
		Id:                               info.GetId(),
		Name:                             info.GetName(),
		Status:                           DomainStatus(info.GetState()),
		Description:                      info.GetDescription(),
		OwnerEmail:                       info.GetOwnerEmail(),
		Data:                             info.GetData(),
		WorkflowExecutionRetentionPeriod: Duration(config.GetWorkflowExecutionRetentionTtl()),
		BadBinaries:                      BadBinaries(config.GetBadBinaries()),
		HistoryArchivalStatus:            ArchivalStatus(config.GetHistoryArchivalState()),
		HistoryArchivalUri:               config.GetVisibilityArchivalUri(),
		VisibilityArchivalStatus:         ArchivalStatus(config.GetVisibilityArchivalState()),
		VisibilityArchivalUri:            config.GetVisibilityArchivalUri(),
		ActiveClusterName:                replicationConfig.GetActiveClusterName(),
		Clusters:                         Clusters(replicationConfig.GetClusters()),
		FailoverVersion:                  failoverVersion,
		IsGlobalDomain:                   true,
		//FailoverInfo:                     nil,
		//IsolationGroups:                  nil,
		//AsyncWorkflowConfig:              nil,
	}
}

func Clusters(clusters []*replication.ClusterReplicationConfig) []*cadence.ClusterReplicationConfiguration {
	if clusters == nil {
		return nil
	}

	result := make([]*cadence.ClusterReplicationConfiguration, 0, len(clusters))
	for _, c := range clusters {
		result = append(result, &cadence.ClusterReplicationConfiguration{
			ClusterName: c.GetClusterName(),
		})
	}
	return result
}

func ArchivalStatus(state enums.ArchivalState) cadence.ArchivalStatus {
	return cadence.ArchivalStatus(state)
}

func BadBinaries(badBinaries *namespace.BadBinaries) *cadence.BadBinaries {
	if badBinaries == nil {
		return nil
	}

	binaries := make(map[string]*cadence.BadBinaryInfo, len(badBinaries.GetBinaries()))
	for k, v := range badBinaries.GetBinaries() {
		binaries[k] = &cadence.BadBinaryInfo{
			Reason:      v.GetReason(),
			Operator:    v.GetOperator(),
			CreatedTime: Timestamp(v.GetCreateTime()),
		}
	}

	return &cadence.BadBinaries{
		Binaries: binaries,
	}
}

func DomainStatus(state enums.NamespaceState) cadence.DomainStatus {
	return cadence.DomainStatus(state)
}

func DomainOperation(operation enumsspb.NamespaceOperation) adminv1.DomainOperation {
	return adminv1.DomainOperation(operation)
}
