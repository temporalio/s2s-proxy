package cadencetype

import (
	"github.com/gogo/protobuf/types"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	temporal "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log"
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

func PollWorkflowTaskQueueResponse(resp *workflowservice.PollWorkflowTaskQueueResponse) *cadence.PollForDecisionTaskResponse {
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
	events := make([]*cadence.HistoryEvent, len(h.GetEvents()))
	for _, e := range h.Events {
		events = append(events, HistoryEvent(e))
	}
	return &cadence.History{Events: events}
}

func HistoryEvent(e *history.HistoryEvent) *cadence.HistoryEvent {
	jsonData, err := protojson.Marshal(e)
	if err != nil {
		log.Fatal("Marshaling error:", err)
	}

	event := &cadence.HistoryEvent{}
	err = event.Unmarshal(jsonData)
	if err != nil {
		log.Fatal("Unmarshaling error: ", err)
	}

	return event
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
