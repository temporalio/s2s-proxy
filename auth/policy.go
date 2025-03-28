package auth

import "slices"

type (
	ActionGroup struct {
		Actions []string
	}
)

var (
	workflowServiceDisallowedAPIs = []string{
		"DeprecateNamespace",
		"ListNamespaces",
		"RegisterNamespace",
		"GetSystemInfo",
	}
)

func IsAllowedWorkflowMigrationAPIs(action string) bool {
	return !slices.Contains(workflowServiceDisallowedAPIs, action)
}
