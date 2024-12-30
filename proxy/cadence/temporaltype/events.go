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
	default:
		panic(fmt.Sprintf("unsupported Cadence event type: %v", event.EventType))
	}

	return temporalEvent
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
			ParentInitiatedEventId:          Int64Ptr(attributes.ParentInitiatedEventID),
			TaskQueue:                       TaskQueue(proto.FromTaskList(attributes.TaskList)),
			Input:                           Payload(proto.FromPayload(attributes.Input)),
			WorkflowExecutionTimeout:        DurationFromSeconds(attributes.GetExecutionStartToCloseTimeoutSeconds()),
			WorkflowTaskTimeout:             DurationFromSeconds(attributes.GetTaskStartToCloseTimeoutSeconds()),
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
			FirstWorkflowTaskBackoff:        DurationFromSeconds(attributes.GetFirstDecisionTaskBackoffSeconds()),
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
