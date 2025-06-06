package auth

import "slices"

var (
	workflowServiceDisallowedAPIs = []string{
		"DeprecateNamespace",
		"RegisterNamespace",
	}
)

func IsAllowedWorkflowMigrationAPIs(action string) bool {
	return !slices.Contains(workflowServiceDisallowedAPIs, action)
}
