package interceptor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
)

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

func TestIsAnyType(t *testing.T) {
	require.False(t, isAnyType(namespaceTranslationSkippableTypes, nil))
	// Response type
	require.True(t, isAnyType(namespaceTranslationSkippableTypes, (*workflowservice.ListWorkflowExecutionsResponse)(nil)))
	require.True(t, isAnyType(namespaceTranslationSkippableTypes, &workflowservice.ListWorkflowExecutionsResponse{}))
	// Request type
	require.False(t, isAnyType(namespaceTranslationSkippableTypes, (*workflowservice.ListWorkflowExecutionsRequest)(nil)))
	require.False(t, isAnyType(namespaceTranslationSkippableTypes, &workflowservice.ListWorkflowExecutionsRequest{}))
}
