package cadence

import (
	"github.com/gogo/protobuf/types"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	temporal "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
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

func toCadenceWorkflowExecution(e *temporal.WorkflowExecution) *cadence.WorkflowExecution {
	return &cadence.WorkflowExecution{
		WorkflowId: e.GetWorkflowId(),
		RunId:      e.GetRunId(),
	}
}

func toCadenceWorkflowType(t *temporal.WorkflowType) *cadence.WorkflowType {
	return &cadence.WorkflowType{
		Name: t.GetName(),
	}
}

func toInt64ValuePtr(i int64) *types.Int64Value {
	return &types.Int64Value{
		Value: i,
	}
}

func toTemporalTaskQueueKind(kind cadence.TaskListKind) enums.TaskQueueKind {
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
