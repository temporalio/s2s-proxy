package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func newTestEmitter(t *testing.T) *Emitter {
	t.Helper()
	e := NewEmitter(log.NewNoopLogger(), CurrentVersion)
	e.SetPackageName("test")
	e.SetFunctionSignature("func visit(vAny any) error")
	e.SetFunctionTrailer("return nil")
	return e
}

func visitByName(t *testing.T, e *Emitter, name protoreflect.FullName) {
	t.Helper()
	mt, err := protoregistry.GlobalTypes.FindMessageByName(name)
	require.NoError(t, err)
	e.Visit(mt)
}

// payloadPathsWithFailurePrune generates with a Payload handler, plus a Failure
// handler that prunes when prune is set, and returns the output.
func payloadPathsWithFailurePrune(t *testing.T, prune bool) string {
	t.Helper()
	e := newTestEmitter(t)
	comment := func(v string, path VisitPath) string { return fmt.Sprintf("_ = %s // %s", v, path) }
	e.AddHandler(func(vt VisitType, _ VisitPath) bool { return vt.GoTypeName() == "Failure" }, prune, comment)
	e.AddHandler(func(vt VisitType, _ VisitPath) bool { return vt.GoTypeName() == "Payload" }, false, comment)
	visitByName(t, e, "temporal.server.api.replication.v1.SyncActivityTaskAttributes")

	var buf bytes.Buffer
	require.NoError(t, e.Generate(&buf))
	return buf.String()
}

func TestEmitterPrunedHandlerStopsDescent(t *testing.T) {
	const underFailure = "SyncActivityTaskAttributes/LastFailure/Failure/EncodedAttributes/Payload"

	require.Contains(t, payloadPathsWithFailurePrune(t, false), underFailure, "control: payload under Failure is found without prune")

	out := payloadPathsWithFailurePrune(t, true)
	require.Contains(t, out, "SyncActivityTaskAttributes/LastFailure/Failure")
	require.NotContains(t, out, underFailure, "a pruned match must not be descended into")
	require.Contains(t, out, "SyncActivityTaskAttributes/Details/Payloads/Payloads/Payload", "other paths are unaffected")
}

func TestEmitterCheckErrorsFailGenerate(t *testing.T) {
	e := newTestEmitter(t)
	e.SetCheck(func(vt VisitType, _ VisitPath, _ bool) (Action, error) {
		if f, ok := vt.AsField(); ok && f.Name() == "last_failure" {
			return Prune, errors.New("rejected " + string(f.FullName()))
		}
		return Descend, nil
	})
	visitByName(t, e, "temporal.server.api.replication.v1.SyncActivityTaskAttributes")

	var buf bytes.Buffer
	err := e.Generate(&buf)
	require.ErrorContains(t, err, "rejected temporal.server.api.replication.v1.SyncActivityTaskAttributes.last_failure")
	require.Zero(t, buf.Len(), "nothing is written when generation fails")
}

func TestEmitterImportsOnlyNamedPackages(t *testing.T) {
	e := newTestEmitter(t)
	e.AddHandler(
		func(vt VisitType, _ VisitPath) bool { return vt.GoTypeName() == "Failure" },
		true,
		func(v string, _ VisitPath) string { return "_ = " + v },
	)
	visitByName(t, e, "temporal.server.api.adminservice.v1.StreamWorkflowReplicationMessagesResponse")

	var buf bytes.Buffer
	require.NoError(t, e.Generate(&buf))
	out := buf.String()
	require.Contains(t, out, `"go.temporal.io/server/api/adminservice/v1"`, "root case type")
	require.NotContains(t, out, `"go.temporal.io/api/failure/v1"`, "only reached through getters, never named")
}

// payloadHandlerOutput generates with a handler on every Payload for the given
// roots and returns the output.
func payloadHandlerOutput(t *testing.T, roots ...protoreflect.FullName) string {
	t.Helper()
	e := newTestEmitter(t)
	e.AddHandler(
		func(vt VisitType, _ VisitPath) bool {
			return vt.GoTypeName() == "Payload" || vt.GoTypeName() == "DataBlob"
		},
		true,
		func(v string, _ VisitPath) string { return "_ = " + v },
	)
	for _, r := range roots {
		visitByName(t, e, r)
	}

	var buf bytes.Buffer
	require.NoError(t, e.Generate(&buf))
	src, err := format.Source(buf.Bytes())
	require.NoError(t, err)
	return string(src)
}

func TestEmitterNamesNestedRootTypes(t *testing.T) {
	out := payloadHandlerOutput(t,
		"temporal.server.api.adminservice.v1.AddTasksRequest.Task",
		"temporal.server.api.adminservice.v1.Task",
	)
	require.Contains(t, out, "case *serverapiadminservice.AddTasksRequest_Task:")
}

// Sibling chains of singular fields share one Go scope, so a variable from the
// first chain must not be handed out again to the second.
func TestEmitterDoesNotRedeclareVariablesInScope(t *testing.T) {
	out := payloadHandlerOutput(t, "temporal.server.api.adminservice.v1.DescribeMutableStateResponse")

	m := regexp.MustCompile(`\n(\t+)(\w+) := root\.GetDatabaseMutableState\(\)`).FindStringSubmatch(out)
	require.NotNil(t, m)
	decl := "\n" + m[1] + m[2] + " := "
	require.Equal(t, 1, strings.Count(out, decl), "%s is declared more than once at the same depth:\n%s", m[2], out)
}
