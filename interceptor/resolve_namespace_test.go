package interceptor

import (
	"reflect"
	"testing"

	"github.com/keilerkonzept/visit"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/history/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
)

// resolveAt walks obj and returns what resolveNamespaceID answers at every field named fieldName.
// Walking for real is the point: it exercises the same parent chain the translator sees.
func resolveAt(t *testing.T, obj any, fieldName, fallback string) []string {
	t.Helper()
	var got []string
	err := visit.Values(obj, func(vwp visit.ValueWithParent) (visit.Action, error) {
		if vwp.Kind() == reflect.Ptr && vwp.IsNil() {
			return visit.Skip, nil
		}
		fieldType, action := getParentFieldType(vwp)
		if action != "" {
			return action, nil
		}
		if fieldType.Name == fieldName {
			got = append(got, resolveNamespaceID(vwp, fallback))
		}
		return visit.Continue, nil
	})
	require.NoError(t, err)
	return got
}

func testSearchAttributes() *common.SearchAttributes {
	return &common.SearchAttributes{
		IndexedFields: map[string]*common.Payload{"TestSA": {Data: []byte("v")}},
	}
}

func TestResolveNamespaceIDOneHop(t *testing.T) {
	// The owner is the immediate parent. This is the common case.
	obj := &persistencespb.WorkflowExecutionInfo{
		NamespaceId:      "ns-a",
		SearchAttributes: map[string]*common.Payload{"TestSA": {Data: []byte("v")}},
	}
	require.Equal(t, []string{"ns-a"}, resolveAt(t, obj, "SearchAttributes", "unused"))
}

func TestResolveNamespaceIDMultipleHops(t *testing.T) {
	// The blob sits two levels below its owner, so checking only the immediate parent is not
	// enough: VersionedTransitionArtifact has no NamespaceId of its own.
	obj := &replicationspb.SyncVersionedTransitionTaskAttributes{
		NamespaceId: "ns-a",
		VersionedTransitionArtifact: &replicationspb.VersionedTransitionArtifact{
			EventBatches: []*common.DataBlob{{Data: []byte("x")}},
			NewRunInfo: &replicationspb.NewRunInfo{
				EventBatch: &common.DataBlob{Data: []byte("y")},
			},
		},
	}
	require.Equal(t, []string{"ns-a"}, resolveAt(t, obj, "EventBatches", "unused"))
	// NewRunInfo puts a blob one level deeper still.
	require.Equal(t, []string{"ns-a"}, resolveAt(t, obj, "EventBatch", "unused"))
}

func TestResolveNamespaceIDIgnoresChildNamespaceID(t *testing.T) {
	// StartChildWorkflowExecutionInitiatedEventAttributes holds the CHILD's NamespaceId right next
	// to the PARENT's SearchAttributes. It is not an owner, so the walk must step over it. This
	// test fails if anyone swaps the type switch for a NamespaceId field name match.
	obj := &history.HistoryEvent{
		Attributes: &history.HistoryEvent_StartChildWorkflowExecutionInitiatedEventAttributes{
			StartChildWorkflowExecutionInitiatedEventAttributes: &history.StartChildWorkflowExecutionInitiatedEventAttributes{
				NamespaceId:      "ns-child",
				SearchAttributes: testSearchAttributes(),
			},
		},
	}
	require.Equal(t, []string{"ns-parent"}, resolveAt(t, obj, "SearchAttributes", "ns-parent"))
}

func TestResolveNamespaceIDIgnoresParentNamespaceID(t *testing.T) {
	// WorkflowExecutionInfo has both. Only NamespaceId is the workflow's own.
	obj := &persistencespb.WorkflowExecutionInfo{
		NamespaceId:       "ns-a",
		ParentNamespaceId: "ns-parent",
		SearchAttributes:  map[string]*common.Payload{"TestSA": {Data: []byte("v")}},
	}
	require.Equal(t, []string{"ns-a"}, resolveAt(t, obj, "SearchAttributes", "unused"))
}

func TestResolveNamespaceIDEmptyOwnerKeepsWalking(t *testing.T) {
	// An owner whose NamespaceId is empty tells us nothing, so the walk carries on outward.
	obj := &replicationspb.SyncVersionedTransitionTaskAttributes{
		NamespaceId: "ns-a",
		VersionedTransitionArtifact: &replicationspb.VersionedTransitionArtifact{
			StateAttributes: &replicationspb.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes{
				SyncWorkflowStateSnapshotAttributes: &replicationspb.SyncWorkflowStateSnapshotAttributes{
					State: &persistencespb.WorkflowMutableState{
						ExecutionInfo: &persistencespb.WorkflowExecutionInfo{
							NamespaceId:      "",
							SearchAttributes: map[string]*common.Payload{"TestSA": {Data: []byte("v")}},
						},
					},
				},
			},
		},
	}
	require.Equal(t, []string{"ns-a"}, resolveAt(t, obj, "SearchAttributes", "unused"))
}

func TestResolveNamespaceIDFallsBackWhenNoOwner(t *testing.T) {
	// Nothing above the attribute owns a namespace, which is what happens inside a data blob and
	// on the raw history responses.
	obj := &history.WorkflowExecutionStartedEventAttributes{SearchAttributes: testSearchAttributes()}
	require.Equal(t, []string{"ns-fallback"}, resolveAt(t, obj, "SearchAttributes", "ns-fallback"))
	require.Equal(t, []string{""}, resolveAt(t, obj, "SearchAttributes", ""))
}
