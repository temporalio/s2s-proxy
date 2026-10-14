package main

import (
	"fmt"
	"strings"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// buildUTF8 returns an emitter for RepairInvalidUTF8, which repairs invalid
// UTF-8 in every Failure reachable from any registered message.
func buildUTF8(logger log.Logger) *Emitter {
	emitter := NewEmitter(logger, Gogo122Version)
	emitter.SetPackageName("compat")
	emitter.SetFunctionSignature(
		`func RepairInvalidUTF8(vAny any) (ret bool, retErr error)`,
	)
	emitter.SetFunctionTrailer("return")
	emitter.AddHandler(
		// Match any type called "Failure"
		func(vt VisitType, path VisitPath) bool {
			// Match Failure types
			if vt.GoTypeName() != "Failure" {
				logger.Debug("ignore non Failure field", tag.NewAnyTag("path", path.String()))
				return false
			}
			// Skip nested "Cause" field in Failure types. The repairInvalidUTF8InFailure handler function
			// will descend into these.
			ps := path.String()
			if strings.Contains(ps, "/Cause") {
				logger.Debug("ignore failure Cause", tag.NewAnyTag("path", path.String()))
				return false
			}

			// These do not have a failure field in Temporal v1.22 (they do in later versions)
			if strings.Contains(ps, "WorkflowQueryResult") ||
				strings.Contains(ps, "RespondQueryTaskCompletedRequest") ||
				strings.Contains(ps, "QueryFailedFailure") {
				return false
			}
			return true
		},
		false,
		// Generate code to handle the Failure field
		func(varName string, _ VisitPath) string {
			return fmt.Sprintf(`if changed, err := repairInvalidUTF8InFailure(%s); err != nil || changed {
				ret = ret || changed
				if err != nil {
					retErr = err
				}
			}`, varName)
		},
	)

	// We traverse the current version of protobuf types (not the gogo-based protos)
	// because protoreflect only works with the current version of protobuf types.
	// The emitter can translate back to gogo-based types, if it is configured with
	// Mode=Gogo122Version.
	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		emitter.Visit(mt)
		return true
	})

	return emitter
}
