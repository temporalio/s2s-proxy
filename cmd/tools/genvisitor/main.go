package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	// Import to populate the protoregistry
	_ "go.temporal.io/api/workflowservice/v1"
	_ "go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

func main() {
	debugFlag := flag.Bool("debug", false, "enable debug logs to stderr")
	dumpTree := flag.Bool("dump-tree", false, "print the tree of matched paths in the type hierarchy to stderr")
	flag.Parse()

	var logger log.Logger
	if *debugFlag {
		logger = log.NewCLILogger()
	} else {
		logger = log.NewNoopLogger()
	}

	emitter := NewEmitter(logger, Gogo122Version)
	emitter.SetFunctionSignature(
		`func repairInvalidUTF8(vAny any) (ret bool, retErr error)`,
	)
	emitter.SetFunctionTrailer("return")
	emitter.AddHandler(
		// Match any type called "Failure"
		func(vt VisitType, path VisitPath) bool {
			// Match Failure types, but not Cause fields.
			if vt.GoTypeName() != "Failure" {
				logger.Debug("ignore non Failure field", tag.NewAnyTag("path", path.String()))
				return false
			}
			if strings.Contains(path.String(), "/Cause") {
				logger.Debug("ignore failure Cause", tag.NewAnyTag("path", path.String()))
				return false
			}
			return true
		},
		// Generate code to handle the Failure field
		func(varName string) string {
			return fmt.Sprintf(`if changed, err := repairInvalidUTF8InFailure(%s); err != nil || changed {
				ret = ret || changed
				if err != nil {
					retErr = err
				}
			}`, varName)
		},
	)

	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		emitter.Visit(mt)
		return true
	})
	if *dumpTree {
		emitter.root.Dump(os.Stderr)
	}
	emitter.Generate(os.Stdout)
}
