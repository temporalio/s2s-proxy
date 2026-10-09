package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const syncActivityDetails = "temporal.server.api.replication.v1.SyncActivityTaskAttributes.details"

func messageDescriptor(t *testing.T, name protoreflect.FullName) protoreflect.MessageDescriptor {
	t.Helper()
	mt, err := protoregistry.GlobalTypes.FindMessageByName(name)
	require.NoError(t, err)
	return mt.Descriptor()
}

// pathsSeen records the path of every node the walk offers.
func pathsSeen(t *testing.T, root protoreflect.FullName, act func(VisitType) Action) []string {
	t.Helper()
	var seen []string
	Visit(messageDescriptor(t, root), func(vt VisitType, path VisitPath) Action {
		seen = append(seen, path.String())
		return act(vt)
	})
	return seen
}

func anyWithPrefix(paths []string, prefix string) bool {
	return slices.ContainsFunc(paths, func(p string) bool { return strings.HasPrefix(p, prefix) })
}

func TestVisitDescendsWithoutPrune(t *testing.T) {
	seen := pathsSeen(t, "temporal.server.api.replication.v1.SyncActivityTaskAttributes", func(VisitType) Action { return Descend })
	require.True(t, anyWithPrefix(seen, "SyncActivityTaskAttributes/Details/"), "control: details has children when not pruned")
}

func TestVisitPruneSkipsSubtreeOnly(t *testing.T) {
	seen := pathsSeen(t, "temporal.server.api.replication.v1.SyncActivityTaskAttributes", func(vt VisitType) Action {
		if vt.FullName() == syncActivityDetails {
			return Prune
		}
		return Descend
	})

	require.Contains(t, seen, "SyncActivityTaskAttributes/Details")
	require.False(t, anyWithPrefix(seen, "SyncActivityTaskAttributes/Details/"), "pruned field's children must not be visited")
	require.True(t, anyWithPrefix(seen, "SyncActivityTaskAttributes/LastFailure"), "siblings after a pruned field must be visited")
}

func TestVisitAbortSkipsLaterSiblings(t *testing.T) {
	seen := pathsSeen(t, "temporal.server.api.replication.v1.SyncActivityTaskAttributes", func(vt VisitType) Action {
		if vt.FullName() == syncActivityDetails {
			return Abort
		}
		return Descend
	})

	require.Contains(t, seen, "SyncActivityTaskAttributes/Details")
	require.False(t, anyWithPrefix(seen, "SyncActivityTaskAttributes/LastFailure"), "Abort keeps the old stop-the-message behavior")
}

func TestVisitReportsCycles(t *testing.T) {
	var cycles []protoreflect.FullName
	Visit(messageDescriptor(t, "temporal.server.api.persistence.v1.StateMachineNode"), func(vt VisitType, _ VisitPath) Action {
		if vt.Cycle {
			cycles = append(cycles, vt.FullName())
		}
		return Descend
	})

	require.Contains(t, cycles, protoreflect.FullName("temporal.server.api.persistence.v1.StateMachineNode"))
}

func TestGoIdent(t *testing.T) {
	require.Equal(t, "ChasmNodeMetadata",
		goIdent(messageDescriptor(t, "temporal.server.api.persistence.v1.ChasmNodeMetadata")))
	require.Equal(t, "ChasmComponentAttributes_Task",
		goIdent(messageDescriptor(t, "temporal.server.api.persistence.v1.ChasmComponentAttributes.Task")))
}
