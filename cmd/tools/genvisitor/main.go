package main

import (
	"fmt"
	"os"

	// Import to populate the protoregistry
	_ "go.temporal.io/api/workflowservice/v1"
	_ "go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func main() {
	emitter := NewEmitter(Gogo122Version)
	emitter.AddImport("go.temporal.io/server/common/log")
	emitter.SetFunctionSignature(
		`func repairInvalidUTF8(logger log.Logger, vAny any) (ret bool)`,
	)
	emitter.SetTrailer("return")
	emitter.AddHandler(
		func(s string) bool {
			return s == "LastFailure"
		},
		func(varName string) string {
			return fmt.Sprintf(`ret = ret || repairUTF8InLastFailure(logger, %s)`, varName)
		},
	)
	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		emitter.Visit(mt)
		return true
	})
	emitter.Generate(os.Stdout)
}
