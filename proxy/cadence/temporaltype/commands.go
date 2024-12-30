package temporaltype

import (
	"context"
	"fmt"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/command/v1"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/workflowservice/v1"
	servercommon "go.temporal.io/server/common"
)

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
