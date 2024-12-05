package temporaltype

import (
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/command/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/query/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
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
) *workflowservice.RespondWorkflowTaskCompletedRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondWorkflowTaskCompletedRequest{
		TaskToken:                  request.GetTaskToken(),
		Commands:                   Commands(request.GetDecisions()),
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

func Commands(decisions []*cadence.Decision) []*command.Command {
	if decisions == nil {
		return nil
	}

	commands := make([]*command.Command, len(decisions))
	for i, decision := range decisions {
		commands[i] = Command(decision)
	}
	return commands
}

func Command(decision *cadence.Decision) *command.Command {
	if decision == nil {
		return nil
	}

	c := &command.Command{}

	switch decision.Attributes.(type) {
	case *cadence.Decision_ScheduleActivityTaskDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK
		c.Attributes = &command.Command_ScheduleActivityTaskCommandAttributes{
			ScheduleActivityTaskCommandAttributes: &command.ScheduleActivityTaskCommandAttributes{},
		}
	case *cadence.Decision_StartTimerDecisionAttributes:
	case *cadence.Decision_CompleteWorkflowExecutionDecisionAttributes:
	case *cadence.Decision_FailWorkflowExecutionDecisionAttributes:
	case *cadence.Decision_RequestCancelActivityTaskDecisionAttributes:
	case *cadence.Decision_CancelTimerDecisionAttributes:
	case *cadence.Decision_CancelWorkflowExecutionDecisionAttributes:
	case *cadence.Decision_RequestCancelExternalWorkflowExecutionDecisionAttributes:
	case *cadence.Decision_RecordMarkerDecisionAttributes:
	case *cadence.Decision_ContinueAsNewWorkflowExecutionDecisionAttributes:
	case *cadence.Decision_StartChildWorkflowExecutionDecisionAttributes:
	case *cadence.Decision_SignalExternalWorkflowExecutionDecisionAttributes:
	case *cadence.Decision_UpsertWorkflowSearchAttributesDecisionAttributes:
	default:
		panic("unknown decision type")
	}

	return c
}
