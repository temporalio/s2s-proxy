package interceptor

import (
	"testing"
)

func BenchmarkVisitNamespace(b *testing.B) {
	variants := []struct {
		testName     string
		inputNSName  string
		outputNSName string
		mapping      map[string]string
	}{
		{
			testName:     "name changed",
			inputNSName:  "orig",
			outputNSName: "orig.cloud",
			mapping:      map[string]string{"orig": "orig.cloud"},
		},
		{
			testName:     "name unchanged",
			inputNSName:  "orig",
			outputNSName: "orig",
			mapping:      map[string]string{"other": "other.cloud"},
		},
		{
			testName:     "empty mapping",
			inputNSName:  "orig",
			outputNSName: "orig",
			mapping:      map[string]string{},
		},
		{
			testName:     "nil mapping",
			inputNSName:  "orig",
			outputNSName: "orig",
			mapping:      nil,
		},
	}
	cases := generateNamespaceObjCases()
	cases = append(cases, generateNamespaceReplicationMessages()...)

	for _, c := range cases {
		b.Run(c.objName, func(b *testing.B) {
			for _, variant := range variants {
				translator := createNameTranslator(variant.mapping)
				b.Run(variant.testName, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						input := c.makeType(variant.inputNSName)

						b.StartTimer()
						visitNamespace(input, translator)
					}
				})
			}
		})
	}
}
