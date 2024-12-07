package cadencetype

import (
	"fmt"
	"github.com/gogo/protobuf/types"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	temporal "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
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

func PollForDecisionTaskResponse(resp *workflowservice.PollWorkflowTaskQueueResponse) *cadence.PollForDecisionTaskResponse {
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
		History:                   History(resp.GetHistory()),
		NextPageToken:             resp.GetNextPageToken(),
		WorkflowExecutionTaskList: TaskList(resp.GetWorkflowExecutionTaskQueue()),
		ScheduledTime:             Timestamp(resp.GetScheduledTime()),
		StartedTime:               Timestamp(resp.GetStartedTime()),
	}
}

func History(h *history.History) *cadence.History {
	if h == nil {
		return nil
	}
	events := make([]*cadence.HistoryEvent, 0, len(h.GetEvents()))
	for _, e := range h.Events {
		events = append(events, HistoryEvent(e))
	}
	return &cadence.History{Events: events}
}

func HistoryEvent(e *history.HistoryEvent) *cadence.HistoryEvent {
	event := &cadence.HistoryEvent{
		EventId:   e.GetEventId(),
		EventTime: Timestamp(e.GetEventTime()),
		Version:   e.GetVersion(),
		TaskId:    e.GetTaskId(),
	}

	switch e.Attributes.(type) {
	case *history.HistoryEvent_WorkflowExecutionStartedEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_WorkflowExecutionStartedEventAttributes).WorkflowExecutionStartedEventAttributes
		event.Attributes = WorkflowExecutionStartedEventAttributes(attributes)
	case *history.HistoryEvent_WorkflowTaskScheduledEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_WorkflowTaskScheduledEventAttributes).WorkflowTaskScheduledEventAttributes
		event.Attributes = WorkflowTaskScheduledEventAttributes(attributes)
	case *history.HistoryEvent_WorkflowTaskStartedEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_WorkflowTaskStartedEventAttributes).WorkflowTaskStartedEventAttributes
		event.Attributes = WorkflowTaskStartedEventAttributes(attributes)
	case *history.HistoryEvent_WorkflowTaskTimedOutEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_WorkflowTaskTimedOutEventAttributes).WorkflowTaskTimedOutEventAttributes
		event.Attributes = WorkflowTaskTimedOutEventAttributes(attributes)
	case *history.HistoryEvent_WorkflowTaskFailedEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_WorkflowTaskFailedEventAttributes).WorkflowTaskFailedEventAttributes
		event.Attributes = WorkflowTaskFailedEventAttributes(attributes)
	case *history.HistoryEvent_WorkflowTaskCompletedEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_WorkflowTaskCompletedEventAttributes).WorkflowTaskCompletedEventAttributes
		event.Attributes = WorkflowTaskCompletedEventAttributes(attributes)
	case *history.HistoryEvent_ActivityTaskScheduledEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_ActivityTaskScheduledEventAttributes).ActivityTaskScheduledEventAttributes
		event.Attributes = ActivityTaskScheduledEventAttributes(attributes)
	case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_ActivityTaskStartedEventAttributes).ActivityTaskStartedEventAttributes
		event.Attributes = ActivityTaskStartedEventAttributes(attributes)
	case *history.HistoryEvent_ActivityTaskCompletedEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_ActivityTaskCompletedEventAttributes).ActivityTaskCompletedEventAttributes
		event.Attributes = ActivityTaskCompletedEventAttributes(attributes)
	case *history.HistoryEvent_ActivityTaskFailedEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_ActivityTaskFailedEventAttributes).ActivityTaskFailedEventAttributes
		event.Attributes = ActivityTaskFailedEventAttributes(attributes)
	case *history.HistoryEvent_ActivityTaskTimedOutEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_ActivityTaskTimedOutEventAttributes).ActivityTaskTimedOutEventAttributes
		event.Attributes = ActivityTaskTimedOutEventAttributes(attributes)
	case *history.HistoryEvent_ActivityTaskCancelRequestedEventAttributes:
		attributes := e.Attributes.(*history.HistoryEvent_ActivityTaskCancelRequestedEventAttributes).ActivityTaskCancelRequestedEventAttributes
		event.Attributes = ActivityTaskCancelRequestedEventAttributes(attributes)
	default:
		fmt.Printf("Liang: event type not converted %T to cadence type\n", e.Attributes)
	}

	return event
}

func ActivityTaskCancelRequestedEventAttributes(attributes *history.ActivityTaskCancelRequestedEventAttributes) *cadence.HistoryEvent_ActivityTaskCancelRequestedEventAttributes {
	return &cadence.HistoryEvent_ActivityTaskCancelRequestedEventAttributes{
		ActivityTaskCancelRequestedEventAttributes: &cadence.ActivityTaskCancelRequestedEventAttributes{
			//ActivityId:                   attributes.GetScheduledEventId(),
			DecisionTaskCompletedEventId: attributes.GetWorkflowTaskCompletedEventId(),
		},
	}
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

func RespondActivityTaskFailedResponse(resp *workflowservice.RespondActivityTaskFailedResponse) *cadence.RespondActivityTaskFailedResponse {
	if resp == nil {
		return nil
	}

	return &cadence.RespondActivityTaskFailedResponse{}
}
