package auth

import "slices"

var (
	workflowServiceDisallowedAPIs = []string{
		"DeprecateNamespace",
		"ListNamespaces",
		"RegisterNamespace",
	}
)

func IsAllowedWorkflowMigrationAPIs(action string) bool {
	return !slices.Contains(workflowServiceDisallowedAPIs, action)
}
