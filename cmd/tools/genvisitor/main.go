package main

import (
	"fmt"
	"os"

	// Import to populate the protoregistry
	_ "go.temporal.io/api/workflowservice/v1"
	_ "go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func main() {
	emitter := NewEmitter()
	emitter.AddHandler(
		"ActivityTaskStartedEventAttributes",
		func(varName string) string {
			return fmt.Sprintf(
				`// repairUTF8(%s)`, varName,
			)
		},
	)
	protoregistry.GlobalTypes.RangeMessages(emitter.Visit)
	emitter.Generate(os.Stdout)
}
