package temporaltype

import (
	"fmt"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	cadencecommon "github.com/uber/cadence/common"
	cadencepersistence "github.com/uber/cadence/common/persistence"
	cadencetypes "github.com/uber/cadence/common/types"
	"github.com/uber/cadence/common/types/mapper/proto"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	temporalpb "go.temporal.io/api/history/v1"
	"go.temporal.io/server/common/persistence/serialization"
)

var cadenceEventsSerializer = cadencepersistence.NewPayloadSerializer()
var temporalEventsSerializer = serialization.NewSerializer()

func Events(events *cadence.DataBlob) *common.DataBlob {
	if events == nil {
		return nil
	}

	switch events.GetEncodingType() {
	case cadence.EncodingType_ENCODING_TYPE_THRIFTRW:
		return convertTHRIFTRWEvents(events)
	default:
		panic(fmt.Sprintf("ensupported Cadence encoding type: %v", events.GetEncodingType()))
	}
}

func convertTHRIFTRWEvents(events *cadence.DataBlob) *common.DataBlob {
	internalEvents, err := cadenceEventsSerializer.DeserializeBatchEvents(
		&cadencepersistence.DataBlob{
			Encoding: cadencecommon.EncodingTypeThriftRW,
			Data:     events.GetData(),
		})
	if err != nil {
		panic(fmt.Sprintf("failed to deserialize cadence events: %v", err))
	}

	temporalEvents := make([]*temporalpb.HistoryEvent, len(internalEvents))
	for i, event := range internalEvents {
		temporalEvents[i] = TemporalEvent(event)
	}
	dataBlob, err := temporalEventsSerializer.SerializeEvents(temporalEvents, enums.ENCODING_TYPE_PROTO3)
	if err != nil {
		panic(fmt.Sprintf("failed to deserialize cadence events: %v", err))
	}

	return dataBlob
}

func TemporalEvent(event *cadencetypes.HistoryEvent) *temporalpb.HistoryEvent {
	if event == nil {
		return nil
	}

	temporalEvent := &temporalpb.HistoryEvent{
		EventId:   event.ID,
		EventTime: TimestampFromInt64(event.Timestamp),
		Version:   event.Version,
		TaskId:    event.TaskID,
		//WorkerMayIgnore: false,
		//Attributes and EventType are set in the switch statement below
		//Attributes:      nil,
		//EventType:       event.EventType,
	}

	switch event.GetEventType() {
	case cadencetypes.EventTypeWorkflowExecutionStarted:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED
		temporalEvent.Attributes = WorkflowExecutionStartedEventAttributes(event.GetWorkflowExecutionStartedEventAttributes())
	case cadencetypes.EventTypeWorkflowExecutionCompleted:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_COMPLETED
		temporalEvent.Attributes = WorkflowExecutionCompletedEventAttributes(event.GetWorkflowExecutionCompletedEventAttributes())
	case cadencetypes.EventTypeDecisionTaskScheduled:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_TASK_SCHEDULED
		temporalEvent.Attributes = WorkflowTaskScheduledEventAttributes(event.GetDecisionTaskScheduledEventAttributes())
	case cadencetypes.EventTypeDecisionTaskStarted:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_TASK_STARTED
		temporalEvent.Attributes = WorkflowTaskStartedEventAttributes(event.GetDecisionTaskStartedEventAttributes())
	case cadencetypes.EventTypeDecisionTaskCompleted:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_TASK_COMPLETED
		temporalEvent.Attributes = WorkflowTaskCompletedEventAttributes(event.GetDecisionTaskCompletedEventAttributes())
	case cadencetypes.EventTypeDecisionTaskTimedOut:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_TASK_TIMED_OUT
		temporalEvent.Attributes = WorkflowTaskTimedOutEventAttributes(event.GetDecisionTaskTimedOutEventAttributes())
	case cadencetypes.EventTypeDecisionTaskFailed:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_TASK_FAILED
		temporalEvent.Attributes = WorkflowTaskFailedEventAttributes(event.GetDecisionTaskFailedEventAttributes())
	case cadencetypes.EventTypeActivityTaskScheduled:
		temporalEvent.EventType = enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED
		temporalEvent.Attributes = ActivityTaskScheduledEventAttributes(event.GetActivityTaskScheduledEventAttributes())
	case cadencetypes.EventTypeActivityTaskStarted:
		temporalEvent.EventType = enums.EVENT_TYPE_ACTIVITY_TASK_STARTED
		temporalEvent.Attributes = ActivityTaskStartedEventAttributes(event.GetActivityTaskStartedEventAttributes())
	case cadencetypes.EventTypeActivityTaskCompleted:
		temporalEvent.EventType = enums.EVENT_TYPE_ACTIVITY_TASK_COMPLETED
		temporalEvent.Attributes = ActivityTaskCompletedEventAttributes(event.GetActivityTaskCompletedEventAttributes())
	case cadencetypes.EventTypeActivityTaskFailed:
		temporalEvent.EventType = enums.EVENT_TYPE_ACTIVITY_TASK_FAILED
		temporalEvent.Attributes = ActivityTaskFailedEventAttributes(event.GetActivityTaskFailedEventAttributes())
	case cadencetypes.EventTypeActivityTaskTimedOut:
		temporalEvent.EventType = enums.EVENT_TYPE_ACTIVITY_TASK_TIMED_OUT
		temporalEvent.Attributes = ActivityTaskTimedOutEventAttributes(event.GetActivityTaskTimedOutEventAttributes())
	case cadencetypes.EventTypeActivityTaskCancelRequested:
		temporalEvent.EventType = enums.EVENT_TYPE_ACTIVITY_TASK_CANCEL_REQUESTED
		temporalEvent.Attributes = ActivityTaskCancelRequestedEventAttributes(event.GetActivityTaskCancelRequestedEventAttributes())
	case cadencetypes.EventTypeActivityTaskCanceled:
		temporalEvent.EventType = enums.EVENT_TYPE_ACTIVITY_TASK_CANCELED
		temporalEvent.Attributes = ActivityTaskCanceledEventAttributes(event.GetActivityTaskCanceledEventAttributes())
	case cadencetypes.EventTypeTimerStarted:
		temporalEvent.EventType = enums.EVENT_TYPE_TIMER_STARTED
		temporalEvent.Attributes = TimerStartedEventAttributes(event.GetTimerStartedEventAttributes())
	case cadencetypes.EventTypeTimerFired:
		temporalEvent.EventType = enums.EVENT_TYPE_TIMER_FIRED
		temporalEvent.Attributes = TimerFiredEventAttributes(event.GetTimerFiredEventAttributes())
	case cadencetypes.EventTypeTimerCanceled:
		temporalEvent.EventType = enums.EVENT_TYPE_TIMER_CANCELED
		temporalEvent.Attributes = TimerCanceledEventAttributes(event.GetTimerCanceledEventAttributes())
	case cadencetypes.EventTypeWorkflowExecutionSignaled:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_SIGNALED
		temporalEvent.Attributes = WorkflowExecutionSignaledEventAttributes(event.GetWorkflowExecutionSignaledEventAttributes())
	case cadencetypes.EventTypeWorkflowExecutionTerminated:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_TERMINATED
		temporalEvent.Attributes = WorkflowExecutionTerminatedEventAttributes(event.GetWorkflowExecutionTerminatedEventAttributes())
	case cadencetypes.EventTypeWorkflowExecutionContinuedAsNew:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_CONTINUED_AS_NEW
		temporalEvent.Attributes = WorkflowExecutionContinuedAsNewEventAttributes(event.GetWorkflowExecutionContinuedAsNewEventAttributes())
	case cadencetypes.EventTypeWorkflowExecutionFailed:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_FAILED
		temporalEvent.Attributes = WorkflowExecutionFailedEventAttributes(event.GetWorkflowExecutionFailedEventAttributes())
	case cadencetypes.EventTypeWorkflowExecutionTimedOut:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_TIMED_OUT
		temporalEvent.Attributes = WorkflowExecutionTimedOutEventAttributes(event.GetWorkflowExecutionTimedOutEventAttributes())
	case cadencetypes.EventTypeWorkflowExecutionCanceled:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_CANCELED
		temporalEvent.Attributes = WorkflowExecutionCanceledEventAttributes(event.GetWorkflowExecutionCanceledEventAttributes())
	case cadencetypes.EventTypeWorkflowExecutionCancelRequested:
		temporalEvent.EventType = enums.EVENT_TYPE_WORKFLOW_EXECUTION_CANCEL_REQUESTED
		temporalEvent.Attributes = WorkflowExecutionCancelRequestedEventAttributes(event.GetWorkflowExecutionCancelRequestedEventAttributes())
	default:
		panic(fmt.Sprintf("unsupported Cadence event type: %v", event.EventType))
	}

	return temporalEvent
}

func WorkflowExecutionCancelRequestedEventAttributes(
	attributes *cadencetypes.WorkflowExecutionCancelRequestedEventAttributes,
) *temporalpb.HistoryEvent_WorkflowExecutionCancelRequestedEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowExecutionCancelRequestedEventAttributes{
		WorkflowExecutionCancelRequestedEventAttributes: &temporalpb.WorkflowExecutionCancelRequestedEventAttributes{
			Cause:                     attributes.Cause,
			ExternalInitiatedEventId:  Int64(attributes.ExternalInitiatedEventID),
			ExternalWorkflowExecution: WorkflowExecution(proto.FromWorkflowExecution(attributes.ExternalWorkflowExecution)),
			Identity:                  attributes.Identity,
		},
	}
}

func WorkflowExecutionCanceledEventAttributes(
	attributes *cadencetypes.WorkflowExecutionCanceledEventAttributes,
) *temporalpb.HistoryEvent_WorkflowExecutionCanceledEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowExecutionCanceledEventAttributes{
		WorkflowExecutionCanceledEventAttributes: &temporalpb.WorkflowExecutionCanceledEventAttributes{
			WorkflowTaskCompletedEventId: attributes.DecisionTaskCompletedEventID,
			Details:                      Payload(proto.FromPayload(attributes.Details)),
		},
	}
}

func WorkflowExecutionTimedOutEventAttributes(
	attributes *cadencetypes.WorkflowExecutionTimedOutEventAttributes,
) *temporalpb.HistoryEvent_WorkflowExecutionTimedOutEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowExecutionTimedOutEventAttributes{
		WorkflowExecutionTimedOutEventAttributes: &temporalpb.WorkflowExecutionTimedOutEventAttributes{
			RetryState:        0,
			NewExecutionRunId: "",
		},
	}
}

func WorkflowExecutionFailedEventAttributes(
	attributes *cadencetypes.WorkflowExecutionFailedEventAttributes,
) *temporalpb.HistoryEvent_WorkflowExecutionFailedEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowExecutionFailedEventAttributes{
		WorkflowExecutionFailedEventAttributes: &temporalpb.WorkflowExecutionFailedEventAttributes{
			Failure:                      Failure(proto.FromFailure(attributes.Reason, attributes.Details)),
			WorkflowTaskCompletedEventId: attributes.DecisionTaskCompletedEventID,
			//RetryState:                   0,
			//NewExecutionRunId:            "",
		},
	}
}

func WorkflowExecutionContinuedAsNewEventAttributes(attributes *cadencetypes.WorkflowExecutionContinuedAsNewEventAttributes) *temporalpb.HistoryEvent_WorkflowExecutionContinuedAsNewEventAttributes {
	panic("WorkflowExecutionContinuedAsNewEventAttributes not implemented")
}

func WorkflowExecutionTerminatedEventAttributes(
	attributes *cadencetypes.WorkflowExecutionTerminatedEventAttributes,
) *temporalpb.HistoryEvent_WorkflowExecutionTerminatedEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowExecutionTerminatedEventAttributes{
		WorkflowExecutionTerminatedEventAttributes: &temporalpb.WorkflowExecutionTerminatedEventAttributes{
			Reason:   attributes.Reason,
			Details:  Payload(proto.FromPayload(attributes.Details)),
			Identity: attributes.Identity,
		},
	}
}

func WorkflowExecutionSignaledEventAttributes(attributes *cadencetypes.WorkflowExecutionSignaledEventAttributes) *temporalpb.HistoryEvent_WorkflowExecutionSignaledEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowExecutionSignaledEventAttributes{
		WorkflowExecutionSignaledEventAttributes: &temporalpb.WorkflowExecutionSignaledEventAttributes{
			SignalName: attributes.SignalName,
			Input:      Payload(proto.FromPayload(attributes.Input)),
			Identity:   attributes.Identity,
			//Header:                    nil,
			//SkipGenerateWorkflowTask:  false,
			//ExternalWorkflowExecution: nil,
		},
	}
}

func TimerCanceledEventAttributes(
	attributes *cadencetypes.TimerCanceledEventAttributes,
) *temporalpb.HistoryEvent_TimerCanceledEventAttributes {
	return &temporalpb.HistoryEvent_TimerCanceledEventAttributes{
		TimerCanceledEventAttributes: &temporalpb.TimerCanceledEventAttributes{
			TimerId:                      attributes.TimerID,
			StartedEventId:               attributes.StartedEventID,
			WorkflowTaskCompletedEventId: attributes.DecisionTaskCompletedEventID,
			Identity:                     attributes.Identity,
		},
	}
}

func TimerFiredEventAttributes(
	attributes *cadencetypes.TimerFiredEventAttributes,
) *temporalpb.HistoryEvent_TimerFiredEventAttributes {
	return &temporalpb.HistoryEvent_TimerFiredEventAttributes{
		TimerFiredEventAttributes: &temporalpb.TimerFiredEventAttributes{
			TimerId:        attributes.TimerID,
			StartedEventId: attributes.StartedEventID,
		},
	}
}

func TimerStartedEventAttributes(
	attributes *cadencetypes.TimerStartedEventAttributes,
) *temporalpb.HistoryEvent_TimerStartedEventAttributes {
	return &temporalpb.HistoryEvent_TimerStartedEventAttributes{
		TimerStartedEventAttributes: &temporalpb.TimerStartedEventAttributes{
			TimerId:                      attributes.TimerID,
			StartToFireTimeout:           DurationFromInt64Seconds(attributes.GetStartToFireTimeoutSeconds()),
			WorkflowTaskCompletedEventId: attributes.DecisionTaskCompletedEventID,
		},
	}
}

func ActivityTaskCanceledEventAttributes(
	attributes *cadencetypes.ActivityTaskCanceledEventAttributes,
) *temporalpb.HistoryEvent_ActivityTaskCanceledEventAttributes {
	return &temporalpb.HistoryEvent_ActivityTaskCanceledEventAttributes{
		ActivityTaskCanceledEventAttributes: &temporalpb.ActivityTaskCanceledEventAttributes{
			Details:                      Payload(proto.FromPayload(attributes.Details)),
			LatestCancelRequestedEventId: attributes.LatestCancelRequestedEventID,
			ScheduledEventId:             attributes.ScheduledEventID,
			StartedEventId:               attributes.StartedEventID,
			Identity:                     attributes.Identity,
			//WorkerVersion:                nil,
		},
	}
}

func ActivityTaskCancelRequestedEventAttributes(attributes *cadencetypes.ActivityTaskCancelRequestedEventAttributes) *temporalpb.HistoryEvent_ActivityTaskCancelRequestedEventAttributes {
	panic("ActivityTaskCancelRequestedEventAttributes not implemented")
	//return &temporalpb.HistoryEvent_ActivityTaskCancelRequestedEventAttributes{
	//	ActivityTaskCancelRequestedEventAttributes: &temporalpb.ActivityTaskCancelRequestedEventAttributes{
	//		ScheduledEventId:             attributes.DecisionTaskCompletedEventID,
	//		//WorkflowTaskCompletedEventId: 0,
	//	},
	//}
}

func ActivityTaskTimedOutEventAttributes(
	attributes *cadencetypes.ActivityTaskTimedOutEventAttributes,
) *temporalpb.HistoryEvent_ActivityTaskTimedOutEventAttributes {
	return &temporalpb.HistoryEvent_ActivityTaskTimedOutEventAttributes{
		ActivityTaskTimedOutEventAttributes: &temporalpb.ActivityTaskTimedOutEventAttributes{
			Failure:          Failure(proto.FromFailure(attributes.LastFailureReason, attributes.Details)),
			ScheduledEventId: attributes.ScheduledEventID,
			StartedEventId:   attributes.StartedEventID,
			//RetryState:       0,
		},
	}
}

func ActivityTaskFailedEventAttributes(
	attributes *cadencetypes.ActivityTaskFailedEventAttributes,
) *temporalpb.HistoryEvent_ActivityTaskFailedEventAttributes {
	return &temporalpb.HistoryEvent_ActivityTaskFailedEventAttributes{
		ActivityTaskFailedEventAttributes: &temporalpb.ActivityTaskFailedEventAttributes{
			Failure:          Failure(proto.FromFailure(attributes.Reason, attributes.Details)),
			ScheduledEventId: attributes.ScheduledEventID,
			StartedEventId:   attributes.StartedEventID,
			Identity:         attributes.Identity,
			//RetryState:       0,
			//WorkerVersion:    nil,
		},
	}
}

func ActivityTaskCompletedEventAttributes(
	attributes *cadencetypes.ActivityTaskCompletedEventAttributes,
) *temporalpb.HistoryEvent_ActivityTaskCompletedEventAttributes {
	return &temporalpb.HistoryEvent_ActivityTaskCompletedEventAttributes{
		ActivityTaskCompletedEventAttributes: &temporalpb.ActivityTaskCompletedEventAttributes{
			Result:           Payload(proto.FromPayload(attributes.Result)),
			ScheduledEventId: attributes.ScheduledEventID,
			StartedEventId:   attributes.StartedEventID,
			Identity:         attributes.Identity,
			//WorkerVersion:    nil,
		},
	}

}

func ActivityTaskStartedEventAttributes(
	attributes *cadencetypes.ActivityTaskStartedEventAttributes,
) *temporalpb.HistoryEvent_ActivityTaskStartedEventAttributes {
	return &temporalpb.HistoryEvent_ActivityTaskStartedEventAttributes{
		ActivityTaskStartedEventAttributes: &temporalpb.ActivityTaskStartedEventAttributes{
			ScheduledEventId: attributes.ScheduledEventID,
			Identity:         attributes.Identity,
			RequestId:        attributes.RequestID,
			Attempt:          attributes.Attempt,
			LastFailure:      Failure(proto.FromFailure(attributes.LastFailureReason, attributes.LastFailureDetails)),
			//WorkerVersion:          nil,
			//BuildIdRedirectCounter: 0,
		},
	}

}

func ActivityTaskScheduledEventAttributes(
	attributes *cadencetypes.ActivityTaskScheduledEventAttributes,
) *temporalpb.HistoryEvent_ActivityTaskScheduledEventAttributes {
	return &temporalpb.HistoryEvent_ActivityTaskScheduledEventAttributes{
		ActivityTaskScheduledEventAttributes: &temporalpb.ActivityTaskScheduledEventAttributes{
			ActivityId:                   attributes.ActivityID,
			ActivityType:                 ActivityType(proto.FromActivityType(attributes.ActivityType)),
			TaskQueue:                    TaskQueue(proto.FromTaskList(attributes.TaskList)),
			Header:                       Header(proto.FromHeader(attributes.Header)),
			Input:                        Payload(proto.FromPayload(attributes.Input)),
			ScheduleToCloseTimeout:       DurationFromInt32Seconds(attributes.GetScheduleToCloseTimeoutSeconds()),
			ScheduleToStartTimeout:       DurationFromInt32Seconds(attributes.GetScheduleToStartTimeoutSeconds()),
			StartToCloseTimeout:          DurationFromInt32Seconds(attributes.GetStartToCloseTimeoutSeconds()),
			HeartbeatTimeout:             DurationFromInt32Seconds(attributes.GetHeartbeatTimeoutSeconds()),
			WorkflowTaskCompletedEventId: attributes.DecisionTaskCompletedEventID,
			RetryPolicy:                  RetryPolicy(proto.FromRetryPolicy(attributes.RetryPolicy)),
			//UseWorkflowBuildId:           false,
		},
	}
}

func WorkflowTaskFailedEventAttributes(
	attributes *cadencetypes.DecisionTaskFailedEventAttributes,
) *temporalpb.HistoryEvent_WorkflowTaskFailedEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowTaskFailedEventAttributes{
		WorkflowTaskFailedEventAttributes: &temporalpb.WorkflowTaskFailedEventAttributes{
			ScheduledEventId: attributes.ScheduledEventID,
			StartedEventId:   attributes.StartedEventID,
			Cause:            WorkflowTaskFailureCause(attributes.GetCause()),
			Failure:          Failure(proto.FromFailure(attributes.Reason, attributes.Details)),
			Identity:         attributes.Identity,
			BaseRunId:        attributes.BaseRunID,
			NewRunId:         attributes.NewRunID,
			ForkEventVersion: attributes.ForkEventVersion,
			BinaryChecksum:   attributes.BinaryChecksum,
			//WorkerVersion:    nil,
		},
	}
}

func WorkflowTaskFailureCause(cause cadencetypes.DecisionTaskFailedCause) enums.WorkflowTaskFailedCause {
	switch cause {
	case cadencetypes.DecisionTaskFailedCauseUnhandledDecision:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_UNHANDLED_COMMAND
	case cadencetypes.DecisionTaskFailedCauseBadScheduleActivityAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_SCHEDULE_ACTIVITY_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadRequestCancelActivityAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_REQUEST_CANCEL_ACTIVITY_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadStartTimerAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_START_TIMER_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadCancelTimerAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_CANCEL_TIMER_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadRecordMarkerAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_RECORD_MARKER_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadCompleteWorkflowExecutionAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_COMPLETE_WORKFLOW_EXECUTION_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadFailWorkflowExecutionAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_FAIL_WORKFLOW_EXECUTION_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadRequestCancelExternalWorkflowExecutionAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_REQUEST_CANCEL_EXTERNAL_WORKFLOW_EXECUTION_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadContinueAsNewAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_CONTINUE_AS_NEW_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadStartChildExecutionAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_START_CHILD_EXECUTION_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadBinary:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_BINARY
	case cadencetypes.DecisionTaskFailedCauseBadCancelWorkflowExecutionAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_CANCEL_WORKFLOW_EXECUTION_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseBadSignalWorkflowExecutionAttributes:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_BAD_SIGNAL_WORKFLOW_EXECUTION_ATTRIBUTES
	case cadencetypes.DecisionTaskFailedCauseWorkflowWorkerUnhandledFailure:
		return enums.WORKFLOW_TASK_FAILED_CAUSE_WORKFLOW_WORKER_UNHANDLED_FAILURE
	default:
		panic(fmt.Sprintf("unsupported Cadence workflow task failure cause: %v", cause))
	}
}

func WorkflowTaskTimedOutEventAttributes(
	attributes *cadencetypes.DecisionTaskTimedOutEventAttributes,
) *temporalpb.HistoryEvent_WorkflowTaskTimedOutEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowTaskTimedOutEventAttributes{
		WorkflowTaskTimedOutEventAttributes: &temporalpb.WorkflowTaskTimedOutEventAttributes{
			ScheduledEventId: attributes.ScheduledEventID,
			StartedEventId:   attributes.StartedEventID,
			TimeoutType:      TimeoutType(attributes.GetTimeoutType()),
		},
	}
}

func TimeoutType(timeoutType cadencetypes.TimeoutType) enums.TimeoutType {
	switch timeoutType {
	case cadencetypes.TimeoutTypeStartToClose:
		return enums.TIMEOUT_TYPE_START_TO_CLOSE
	case cadencetypes.TimeoutTypeScheduleToStart:
		return enums.TIMEOUT_TYPE_SCHEDULE_TO_START
	case cadencetypes.TimeoutTypeScheduleToClose:
		return enums.TIMEOUT_TYPE_SCHEDULE_TO_CLOSE
	case cadencetypes.TimeoutTypeHeartbeat:
		return enums.TIMEOUT_TYPE_HEARTBEAT
	default:
		panic(fmt.Sprintf("unsupported Cadence timeout type: %v", timeoutType))
	}
}

func WorkflowTaskCompletedEventAttributes(
	attributes *cadencetypes.DecisionTaskCompletedEventAttributes,
) *temporalpb.HistoryEvent_WorkflowTaskCompletedEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowTaskCompletedEventAttributes{
		WorkflowTaskCompletedEventAttributes: &temporalpb.WorkflowTaskCompletedEventAttributes{
			ScheduledEventId: attributes.ScheduledEventID,
			StartedEventId:   attributes.StartedEventID,
			Identity:         attributes.Identity,
			BinaryChecksum:   attributes.GetBinaryChecksum(),
			//WorkerVersion:    nil,
			//SdkMetadata:      nil,
			//MeteringMetadata: nil,
		},
	}

}

func WorkflowTaskStartedEventAttributes(
	attributes *cadencetypes.DecisionTaskStartedEventAttributes,
) *temporalpb.HistoryEvent_WorkflowTaskStartedEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowTaskStartedEventAttributes{
		WorkflowTaskStartedEventAttributes: &temporalpb.WorkflowTaskStartedEventAttributes{
			ScheduledEventId: attributes.ScheduledEventID,
			Identity:         attributes.Identity,
			RequestId:        attributes.RequestID,
			//SuggestContinueAsNew:   false,
			//HistorySizeBytes:       0,
			//WorkerVersion:          nil,
			//BuildIdRedirectCounter: 0,
		},
	}

}

func WorkflowTaskScheduledEventAttributes(
	attributes *cadencetypes.DecisionTaskScheduledEventAttributes,
) *temporalpb.HistoryEvent_WorkflowTaskScheduledEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowTaskScheduledEventAttributes{
		WorkflowTaskScheduledEventAttributes: &temporalpb.WorkflowTaskScheduledEventAttributes{
			TaskQueue:           TaskQueue(proto.FromTaskList(attributes.TaskList)),
			StartToCloseTimeout: DurationFromInt32Seconds(attributes.GetStartToCloseTimeoutSeconds()),
			Attempt:             int32(attributes.Attempt),
		},
	}
}

func WorkflowExecutionStartedEventAttributes(
	attributes *cadencetypes.WorkflowExecutionStartedEventAttributes,
) *temporalpb.HistoryEvent_WorkflowExecutionStartedEventAttributes {
	parentWorkflowDomainID := ""
	if attributes.ParentWorkflowDomainID != nil {
		parentWorkflowDomainID = *attributes.ParentWorkflowDomainID
	}
	return &temporalpb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
		WorkflowExecutionStartedEventAttributes: &temporalpb.WorkflowExecutionStartedEventAttributes{
			WorkflowType:                    WorkflowType(proto.FromWorkflowType(attributes.WorkflowType)),
			ParentWorkflowNamespace:         attributes.GetParentWorkflowDomain(),
			ParentWorkflowNamespaceId:       parentWorkflowDomainID,
			ParentWorkflowExecution:         WorkflowExecution(proto.FromWorkflowExecution(attributes.ParentWorkflowExecution)),
			ParentInitiatedEventId:          Int64(attributes.ParentInitiatedEventID),
			TaskQueue:                       TaskQueue(proto.FromTaskList(attributes.TaskList)),
			Input:                           Payload(proto.FromPayload(attributes.Input)),
			WorkflowExecutionTimeout:        DurationFromInt32Seconds(attributes.GetExecutionStartToCloseTimeoutSeconds()),
			WorkflowTaskTimeout:             DurationFromInt32Seconds(attributes.GetTaskStartToCloseTimeoutSeconds()),
			ContinuedExecutionRunId:         "",
			Initiator:                       Initiator(attributes.GetInitiator()),
			ContinuedFailure:                Failure(proto.FromFailure(attributes.ContinuedFailureReason, attributes.ContinuedFailureDetails)),
			LastCompletionResult:            Payload(proto.FromPayload(attributes.LastCompletionResult)),
			OriginalExecutionRunId:          attributes.OriginalExecutionRunID,
			Identity:                        attributes.Identity,
			FirstExecutionRunId:             attributes.FirstExecutionRunID,
			RetryPolicy:                     RetryPolicy(proto.FromRetryPolicy(attributes.RetryPolicy)),
			Attempt:                         attributes.Attempt,
			WorkflowExecutionExpirationTime: TimestampFromInt64(attributes.ExpirationTimestamp),
			CronSchedule:                    attributes.GetCronSchedule(),
			FirstWorkflowTaskBackoff:        DurationFromInt32Seconds(attributes.GetFirstDecisionTaskBackoffSeconds()),
			Memo:                            Memo(proto.FromMemo(attributes.GetMemo())),
			SearchAttributes:                SearchAttributes(proto.FromSearchAttributes(attributes.GetSearchAttributes())),
			PrevAutoResetPoints:             ResetPoints(proto.FromResetPoints(attributes.GetPrevAutoResetPoints())),
			Header:                          Header(proto.FromHeader(attributes.Header)),
			//WorkflowRunTimeout:              nil,
			//ParentInitiatedEventVersion:     0,
			//WorkflowId:                      "",
			//SourceVersionStamp:              nil,
			//CompletionCallbacks:             nil,
			//RootWorkflowExecution:           nil,
			//InheritedBuildId:                "",
		},
	}
}

func WorkflowExecutionCompletedEventAttributes(
	attributes *cadencetypes.WorkflowExecutionCompletedEventAttributes,
) *temporalpb.HistoryEvent_WorkflowExecutionCompletedEventAttributes {
	return &temporalpb.HistoryEvent_WorkflowExecutionCompletedEventAttributes{
		WorkflowExecutionCompletedEventAttributes: &temporalpb.WorkflowExecutionCompletedEventAttributes{
			Result:                       Payload(proto.FromPayload(attributes.Result)),
			WorkflowTaskCompletedEventId: attributes.DecisionTaskCompletedEventID,
			//NewExecutionRunId:            "",
		},
	}
}
