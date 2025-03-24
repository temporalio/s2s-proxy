package config

type (
	ActionGroup struct {
		Actions []string
	}
)

var (
	workflowServiceStarterActions = []string{
		"StartWorkflowExecution",
		"TerminateWorkflowExecution",
		"RequestCancelWorkflowExecution",
		"SignalWithStartWorkflowExecution",
		"SignalWorkflowExecution",
		"ResetWorkflowExecution",
		"ResetStickyTaskQueue",
		"CreateSchedule",
		"UpdateSchedule",
		"PatchSchedule",
		"DeleteSchedule",
		"StartBatchOperation",
		"StopBatchOperation",
		"DeleteWorkflowExecution",
		// update workflow apis
		"UpdateWorkflowExecution",
		"PollWorkflowExecutionUpdate",
		"SetWorkerDeploymentCurrentVersion",
		"SetWorkerDeploymentRampingVersion",
		"DeleteWorkerDeployment",
		"DeleteWorkerDeploymentVersion",
		"UpdateWorkerDeploymentVersionMetadata",
		// activity commands
		"UpdateActivityOptions",
		"PauseActivity",
		"UnpauseActivity",
		"ResetActivity",
	}
	workflowServiceWorkerActions = []string{
		"ShutdownWorker",
		"PollWorkflowTaskQueue",
		"RespondWorkflowTaskCompleted",
		"RespondWorkflowTaskFailed",
		"PollActivityTaskQueue",
		"RecordActivityTaskHeartbeat",
		"RecordActivityTaskHeartbeatById",
		"RespondActivityTaskCompleted",
		"RespondActivityTaskCompletedById",
		"RespondActivityTaskFailed",
		"RespondActivityTaskFailedById",
		"RespondActivityTaskCanceled",
		"RespondActivityTaskCanceledById",
		"RespondQueryTaskCompleted",
	}
	workflowServiceReadActions = []string{
		"DescribeNamespace",
		"GetWorkflowExecutionHistory",
		"GetWorkflowExecutionHistoryReverse",
		"QueryWorkflow",
		"ListOpenWorkflowExecutions",
		"ListClosedWorkflowExecutions",
		"ListWorkflowExecutions",
		"CountWorkflowExecutions",
		"GetSearchAttributes",
		"DescribeWorkflowExecution",
		"DescribeTaskQueue",
		"ListTaskQueuePartitions",
		"DescribeSchedule",
		"ListScheduleMatchingTimes",
		"ListSchedules",
		"DescribeBatchOperation",
		"ListBatchOperations",
		"GetWorkerBuildIdCompatibility",
		"GetWorkerTaskReachability",
		"GetWorkerVersioningRules",
		"DescribeWorkerDeployment",
		"DescribeWorkerDeploymentVersion",
		"ListWorkerDeployments",
	}

	AllowedWorkflowActionGroup = ActionGroup{
		Actions: appendActions(
			workflowServiceReadActions,
			workflowServiceStarterActions,
			workflowServiceWorkerActions,
		),
	}
)

func appendActions(actionsLists ...[]string) []string {
	var res []string
	seen := map[string]bool{}
	for _, actionList := range actionsLists {
		for _, action := range actionList {
			if seen[action] {
				continue
			}
			seen[action] = true
			res = append(res, action)
		}
	}
	return res
}

func IsAllowedWorkflowAction(action string) bool {
	for _, ag := range AllowedWorkflowActionGroup.Actions {
		if ag == action {
			return true
		}
	}
	return false
}
