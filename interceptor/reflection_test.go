package interceptor

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/history/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
)

// TestNamespaceIDOwnersAreValid guards the allowlist against an upstream proto rename and,
// more importantly, against anyone replacing it with a NamespaceId field-name match.
func TestNamespaceIDOwnersAreValid(t *testing.T) {
	expectedOwners := []reflect.Type{
		reflect.TypeFor[replicationspb.HistoryTaskAttributes](),
		reflect.TypeFor[replicationspb.BackfillHistoryTaskAttributes](),
		reflect.TypeFor[replicationspb.SyncVersionedTransitionTaskAttributes](),
		reflect.TypeFor[persistencespb.WorkflowExecutionInfo](),
	}
	require.Len(t, namespaceIDOwners, len(expectedOwners))

	for _, ownerType := range expectedOwners {
		fieldIdx, ok := namespaceIDOwners[ownerType]
		require.True(t, ok, "%v is missing from namespaceIDOwners", ownerType)

		field := ownerType.Field(fieldIdx)
		require.Equal(t, namespaceIDFieldName, field.Name, "owner %v", ownerType)
		require.Equal(t, reflect.String, field.Type.Kind(), "owner %v", ownerType)
	}

	// StartChildWorkflowExecutionInitiatedEventAttributes carries the *child's* NamespaceId
	// alongside its own SearchAttributes. Treating it as an owner would translate a parent's
	// history event with the child's mapping.
	require.NotContains(t, namespaceIDOwners,
		reflect.TypeFor[history.StartChildWorkflowExecutionInitiatedEventAttributes]())
}

func BenchmarkVisitNamespace(b *testing.B) {
	variants := []struct {
		testName    string
		inputNSName string
		mapping     map[string]string
	}{
		{
			testName:    "name changed",
			inputNSName: "orig",
			mapping:     map[string]string{"orig": "orig.cloud"},
		},
		{
			testName:    "name unchanged",
			inputNSName: "orig",
			mapping:     map[string]string{"other": "other.cloud"},
		},
	}
	cases := generateNamespaceObjCases()

	logger := log.NewTestLogger()
	for _, c := range cases {
		b.Run(c.objName, func(b *testing.B) {
			for _, variant := range variants {
				translator := createStringMatcher(variant.mapping)
				b.Run(variant.testName, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						input := c.makeType(variant.inputNSName)

						b.StartTimer()
						_, _ = visitNamespace(logger, input, translator)
					}
				})
			}
		})
	}
}

func BenchmarkVisitSearchAttributes(b *testing.B) {
	variants := []struct {
		testName    string
		inputSAName string
		mapping     map[string]string
	}{
		{
			testName:    "name changed",
			inputSAName: "orig",
			mapping:     map[string]string{"orig": "orig.cloud"},
		},
		{
			testName:    "name unchanged",
			inputSAName: "orig",
			mapping:     map[string]string{"other": "other.cloud"},
		},
	}
	// Includes the deeply nested SyncVersionedTransitionTaskAttributes case, where the namespace
	// owner sits several hops above the search attributes.
	cases := generateSearchAttributeObjs()

	logger := log.NewTestLogger()
	for _, c := range cases {
		b.Run(c.objName, func(b *testing.B) {
			for _, variant := range variants {
				resolve := constMatcherResolver(createStringMatcher(variant.mapping))
				b.Run(variant.testName, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						input := c.makeType(variant.inputSAName)

						b.StartTimer()
						_, _ = visitSearchAttributes(logger, input, resolve, "")
					}
				})
			}
		})
	}
}
