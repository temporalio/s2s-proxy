package cadencetype

import (
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

type CadenceWorkflowExecution struct {
	cadence.WorkflowExecution
}

func (e *CadenceWorkflowExecution) toTemporal() temporal.WorkflowExecution {
	return temporal.WorkflowExecution{
		WorkflowId: e.GetWorkflowId(),
		RunId:      e.GetRunId(),
	}
}

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
		event.Attributes = &cadence.HistoryEvent_WorkflowExecutionStartedEventAttributes{
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

	return event
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

func RespondDecisionTaskCompletedResponse(resp *workflowservice.RespondWorkflowTaskCompletedResponse) *cadence.RespondDecisionTaskCompletedResponse {
	if resp == nil {
		return nil
	}

	return &cadence.RespondDecisionTaskCompletedResponse{}
}
