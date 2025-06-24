package interceptor

import (
	"testing"
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

	for _, c := range cases {
		b.Run(c.objName, func(b *testing.B) {
			for _, variant := range variants {
				visitor := NewNamespaceVisitor(createStringMatcher(variant.mapping))
				b.Run(variant.testName, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						input := c.makeType(variant.inputNSName)

						b.StartTimer()
						_, _ = visitor.Visit(input)
					}
				})
			}
		})
	}
}
