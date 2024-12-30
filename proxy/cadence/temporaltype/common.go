package temporaltype

import (
	"fmt"
	"github.com/gogo/protobuf/types"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	cadencetypes "github.com/uber/cadence/common/types"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/sdk/converter"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func WorkflowType(workflowType *cadence.WorkflowType) *common.WorkflowType {
	if workflowType == nil {
		return nil
	}

	return &common.WorkflowType{
		Name: workflowType.Name,
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

func StickyAttributes(attributes *cadence.StickyExecutionAttributes) *taskqueue.StickyExecutionAttributes {
	return nil
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

func DurationFromSeconds(seconds int32) *durationpb.Duration {
	return &durationpb.Duration{
		Seconds: int64(seconds),
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

func WorkflowExecution(execution *cadence.WorkflowExecution) *common.WorkflowExecution {
	if execution == nil {
		return nil
	}

	return &common.WorkflowExecution{
		WorkflowId: execution.GetWorkflowId(),
		RunId:      execution.GetRunId(),
	}
}

func Timestamp(timestamp *types.Timestamp) *timestamppb.Timestamp {
	if timestamp == nil {
		return nil
	}

	return &timestamppb.Timestamp{
		Seconds: timestamp.GetSeconds(),
		Nanos:   timestamp.GetNanos(),
	}
}

func Int64Value(id *types.Int64Value) int64 {
	if id == nil {
		return 0
	}

	return id.GetValue()
}

func Int64Ptr(id *int64) int64 {
	if id == nil {
		return 0
	}
	return *id
}

func TimestampFromInt64(timestamp *int64) *timestamppb.Timestamp {
	if timestamp == nil {
		return nil
	}

	return &timestamppb.Timestamp{
		Seconds: *timestamp / 1000,
		Nanos:   int32((*timestamp % 1000) * 1000000),
	}
}

func SearchAttributes(attributes *cadence.SearchAttributes) *common.SearchAttributes {
	return nil
}

func Memo(memo *cadence.Memo) *common.Memo {
	return nil
}

func Header(header *cadence.Header) *common.Header {
	return nil
}

func ResetPoints(points *cadence.ResetPoints) *workflow.ResetPoints {
	return nil
}

func Initiator(o cadencetypes.ContinueAsNewInitiator) enums.ContinueAsNewInitiator {
	switch o {
	case cadencetypes.ContinueAsNewInitiatorRetryPolicy:
		return enums.CONTINUE_AS_NEW_INITIATOR_RETRY
	case cadencetypes.ContinueAsNewInitiatorCronSchedule:
		return enums.CONTINUE_AS_NEW_INITIATOR_CRON_SCHEDULE
	case cadencetypes.ContinueAsNewInitiatorDecider:
		return enums.CONTINUE_AS_NEW_INITIATOR_WORKFLOW
	default:
		panic("unknown ContinueAsNewInitiator " + o.String())
		return enums.CONTINUE_AS_NEW_INITIATOR_UNSPECIFIED
	}
}
