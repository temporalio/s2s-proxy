package main

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

var invalidImportAliasChars = regexp.MustCompile("[^a-zA-Z0-9]")

func shouldIgnoreTypeIfDoesntExistIn122(mt protoreflect.Descriptor) bool {
	fullName := string(mt.FullName())
	fullNameLower := strings.ToLower(fullName)
	return strings.Contains(fullNameLower, "nexus") ||
		strings.Contains(fullName, "ExecuteMultiOperation") ||
		strings.Contains(fullName, "SyncWorkflowStateResponse") ||
		strings.Contains(fullName, "SyncWorkflowStateSnapshotAttributes") ||
		strings.Contains(fullName, "SyncWorkflowStateMutationAttributes") ||
		strings.Contains(fullName, "VersionedTransition") ||
		strings.Contains(fullName, "HSMCompletionCallbackArg") ||
		strings.Contains(fullName, "WorkflowMutableStateMutation") ||
		strings.Contains(fullName, "Callback") ||
		strings.Contains(fullName, "Deployment") ||
		strings.HasPrefix(fullName, "temporal.api.export.v1")
}

// getImportAlias returns an import alias for a Go import string.
// ex: Given "go.temporal.io/server/api/adminservice/v1"
// this would return "serverapiadminservicev1".
func getImportAlias(imp string) string {
	parts := strings.Split(imp, "/")                            // Split on '/'
	alias := strings.Join(parts[1:], "")                        // Discard first part of path ("go.temporal.io")
	alias = invalidImportAliasChars.ReplaceAllString(alias, "") // Remove invalid chars
	alias = strings.ReplaceAll(alias, "v1", "")                 // Remove "v1" from the import (bit cleaner)
	return alias
}

func replaceWith122Import(s string) string {
	return strings.ReplaceAll(s, "go.temporal.io", "github.com/temporalio/s2s-proxy/proto/1_22")
}

func writeln(out io.Writer, args ...any) {
	_, err := fmt.Fprintln(out, args...)
	if err != nil {
		panic(err)
	}
}

func writef(out io.Writer, msg string, args ...any) {
	_, err := fmt.Fprintf(out, msg, args...)
	if err != nil {
		panic(err)
	}
}

func write(out io.Writer, args ...any) {
	_, err := fmt.Fprint(out, args...)
	if err != nil {
		panic(err)
	}
}
