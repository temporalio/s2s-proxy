package main

import "strings"

func shouldIgnoreTypeIfDoesntExistIn122(s string) bool {
	return strings.Contains(s, "ExecuteMultiOperation") ||
		strings.Contains(s, "SyncWorkflowStateResponse") ||
		strings.Contains(s, "SyncWorkflowStateSnapshotAttributes") ||
		strings.Contains(s, "VersionedTransition") ||
		strings.Contains(s, "HSMCompletionCallbackArg")
}

func replaceWith122Import(s string) string {
	return strings.ReplaceAll(s, "go.temporal.io",
		"github.com/temporalio/s2s-proxy/common/proto/1_22",
	)
}
