package main

import (
	"maps"
	"os"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// cloneTables copies t so a test can edit it without touching the real tables.
func cloneTables(t payloadTables) payloadTables {
	return payloadTables{
		EventBlobs: maps.Clone(t.EventBlobs),
		Opaque:     maps.Clone(t.Opaque),
		Helpers:    maps.Clone(t.Helpers),
		Ignored:    maps.Clone(t.Ignored),

		DecodeTargets: slices.Clone(t.DecodeTargets),
	}
}

func TestBuildPayloadsSucceedsWithRealTables(t *testing.T) {
	_, err := buildPayloads(log.NewNoopLogger(), adminPayloadTables)
	require.NoError(t, err)
}

// Decode targets are walked as roots of their own, so the helper that decodes a
// blob into one can hand it back to visitAdminPayloads.
func TestBuildPayloadsWalksDecodeTargets(t *testing.T) {
	e, err := buildPayloads(log.NewNoopLogger(), adminPayloadTables)
	require.NoError(t, err)

	var roots []protoreflect.FullName
	for _, vt := range e.root.SortedTypes() {
		roots = append(roots, vt.FullName())
	}
	require.Contains(t, roots, protoreflect.FullName("temporal.server.api.persistence.v1.TransferTaskInfo"))
}

func TestBuildPayloadsRejectsUnknownDecodeTarget(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	tables.DecodeTargets = append(tables.DecodeTargets, "temporal.server.api.persistence.v1.NopeTaskInfo")

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, "decode target temporal.server.api.persistence.v1.NopeTaskInfo")
}

func TestBuildPayloadsRejectsUnclassifiedField(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	delete(tables.Opaque, "temporal.server.api.replication.v1.ReplicationTask.data")

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, "unclassified field temporal.server.api.replication.v1.ReplicationTask.data")
}

func TestBuildPayloadsRejectsStaleEntry(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	tables.Ignored["temporal.server.api.replication.v1.ReplicationTask.nope"] = "typo"

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, "temporal.server.api.replication.v1.ReplicationTask.nope never matched")
}

func TestBuildPayloadsRejectsUnhandledRecursion(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	delete(tables.Helpers, "temporal.server.api.persistence.v1.StateMachineNode")

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, "unhandled recursion: temporal.server.api.persistence.v1.StateMachineNode")
}

func TestBuildPayloadsRejectsOverlappingTables(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	const f protoreflect.FullName = "temporal.server.api.replication.v1.ReplicationTask.data"
	tables.EventBlobs[f] = true

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, string(f)+" is in more than one table")
}

// A recursive type is only safe to cut short when nothing below it would
// generate code. Making one of ReplicationTaskInfo's bytes fields opaque means
// deeper task_equivalents would need visiting too, so the recursion is refused.
func TestBuildPayloadsRejectsRecursionAboveOpaqueData(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	const f protoreflect.FullName = "temporal.server.api.persistence.v1.ReplicationTaskInfo.branch_token"
	delete(tables.Ignored, f)
	tables.Opaque[f] = true

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, "unhandled recursion: temporal.server.api.persistence.v1.ReplicationTaskInfo")
}

func TestGeneratePayloadsMatchesCheckedIn(t *testing.T) {
	got, err := generate(log.NewNoopLogger(), "payloads", nil)
	require.NoError(t, err)

	want, err := os.ReadFile("../../../interceptor/payload_visitor_gen.go")
	require.NoError(t, err)
	require.Equal(t, string(want), string(got), "run make genvisitor-payloads")
}

// A helper only reads the fields it declares. Any other field that could hold
// data would be skipped, so generation must refuse.
func TestBuildPayloadsRejectsUnhandledFieldOfHelperType(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	const n protoreflect.FullName = "temporal.server.api.persistence.v1.StateMachineNode"
	h := tables.Helpers[n]
	h.Handles = slices.DeleteFunc(slices.Clone(h.Handles), func(f protoreflect.FullName) bool { return f == n+".data" })
	tables.Helpers[n] = h

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, "temporal.server.api.persistence.v1.StateMachineNode.data is not handled by visitStateMachineNode")
}

// A task blob only decodes alongside its category, so the whole task goes to a
// helper, which has to keep reading the blob.
func TestBuildPayloadsRejectsAddTasksHelperSkippingBlob(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	const n protoreflect.FullName = "temporal.server.api.adminservice.v1.AddTasksRequest.Task"
	h := tables.Helpers[n]
	h.Handles = slices.DeleteFunc(slices.Clone(h.Handles), func(f protoreflect.FullName) bool { return f == n+".blob" })
	tables.Helpers[n] = h

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, "temporal.server.api.adminservice.v1.AddTasksRequest.Task.blob is not handled by visitAddTasksRequestTask")
}

func TestBuildPayloadsRejectsStaleHelperHandles(t *testing.T) {
	tables := cloneTables(adminPayloadTables)
	const n protoreflect.FullName = "temporal.server.api.persistence.v1.StateMachineNode"
	h := tables.Helpers[n]
	h.Handles = append(slices.Clone(h.Handles), n+".nope")
	tables.Helpers[n] = h

	_, err := buildPayloads(log.NewNoopLogger(), tables)
	require.ErrorContains(t, err, "temporal.server.api.persistence.v1.StateMachineNode.nope never matched")
}
