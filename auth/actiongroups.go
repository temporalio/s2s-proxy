package auth

type (
	ActionGroup struct {
		Actions []string
	}
)

var (
	// APIs matche globalAPIResponses in https://github.com/temporalio/temporal/blob/main/common/rpc/interceptor/redirection.go
	workflowServiceGlobalAPIs = []string{
		"DescribeTaskQueue",
		"DescribeWorkflowExecution",
		"GetWorkflowExecutionHistory",
		"GetWorkflowExecutionHistoryReverse",
		"ListArchivedWorkflowExecutions",
		"ListClosedWorkflowExecutions",
		"ListOpenWorkflowExecutions",
		"ListWorkflowExecutions",
		"ScanWorkflowExecutions",
		"CountWorkflowExecutions",
		"PollActivityTaskQueue",
		"PollWorkflowTaskQueue",
		"PollNexusTaskQueue",
		"QueryWorkflow",
		"RecordActivityTaskHeartbeat",
		"RecordActivityTaskHeartbeatById",
		"RequestCancelWorkflowExecution",
		"ResetStickyTaskQueue",
		"ShutdownWorker",
		"ResetWorkflowExecution",
		"RespondActivityTaskCanceled",
		"RespondActivityTaskCanceledById",
		"RespondActivityTaskCompleted",
		"RespondActivityTaskCompletedById",
		"RespondActivityTaskFailed",
		"RespondActivityTaskFailedById",
		"RespondWorkflowTaskCompleted",
		"RespondWorkflowTaskFailed",
		"RespondQueryTaskCompleted",
		"RespondNexusTaskCompleted",
		"RespondNexusTaskFailed",
		"SignalWithStartWorkflowExecution",
		"SignalWorkflowExecution",
		"StartWorkflowExecution",
		"ExecuteMultiOperation",
		"UpdateWorkflowExecution",
		"PollWorkflowExecutionUpdate",
		"TerminateWorkflowExecution",
		"DeleteWorkflowExecution",
		"ListTaskQueuePartitions",

		"CreateSchedule",
		"DescribeSchedule",
		"UpdateSchedule",
		"PatchSchedule",
		"DeleteSchedule",
		"ListSchedules",
		"ListScheduleMatchingTimes",
		"UpdateWorkerBuildIdCompatibility",
		"GetWorkerBuildIdCompatibility",
		"UpdateWorkerVersioningRules",
		"GetWorkerVersioningRules",
		"GetWorkerTaskReachability",

		"StartBatchOperation",
		"StopBatchOperation",
		"DescribeBatchOperation",
		"ListBatchOperations",
		"UpdateActivityOptions",
		"PauseActivity",
		"UnpauseActivity",
		"ResetActivity",
		"UpdateWorkflowExecutionOptions",

		"DescribeDeployment",
		"ListDeployments",
		"GetDeploymentReachability",
		"GetCurrentDeployment",
		"SetCurrentDeployment",
		"DescribeWorkerDeployment",
		"DescribeWorkerDeploymentVersion",
		"SetWorkerDeploymentCurrentVersion",
		"SetWorkerDeploymentRampingVersion",
		"ListWorkerDeployments",
		"DeleteWorkerDeployment",
		"DeleteWorkerDeploymentVersion",
		"UpdateWorkerDeploymentVersionMetadata",
	}

	AllowedWorkflowActionGroup = ActionGroup{
		Actions: appendActions(
			workflowServiceGlobalAPIs,
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

// These are the set of workflow actions allowed to be forwarded between servers.
func IsAllowedWorkflowAction(action string) bool {
	for _, ag := range AllowedWorkflowActionGroup.Actions {
		if ag == action {
			return true
		}
	}
	return false
}
