package main

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type (
	// Action tells Visit what to do once a node has been offered to the visitor.
	Action int

	visitor func(VisitType, VisitPath) Action

	VisitPath []VisitType
)

const (
	// Descend carries on into the node's children.
	Descend Action = iota
	// Prune skips the node's children and carries on with its siblings.
	Prune
	// Abort skips the node's children and, when the node is a field, every field
	// after it in the same message. It is what returning false used to mean, and
	// the UTF-8 target still depends on it.
	Abort
)

func Visit(obj protoreflect.MessageDescriptor, fn visitor) {
	seen := make(map[string]struct{})
	visit(seen, nil, VisitType{Descriptor: obj}, fn)
}

func visit(
	seen map[string]struct{},
	path VisitPath,
	obj VisitType,
	fn visitor,
) {
	// Key by `ParentType.FieldNameOrTypeName`
	//
	// Mark seen only for this sub-tree so that we visit each field once on a given sub-path.
	seenKey := fmt.Sprintf("%s.%s", obj.Parent().Name(), obj.GoName())
	if _, ok := seen[seenKey]; ok {
		// A message already on this path. Static code cannot unroll it, so tell
		// the visitor and stop; whatever it returns is ignored.
		cycle := VisitType{Descriptor: obj, Cycle: true}
		fn(cycle, append(path, cycle))
		return
	}
	seen[seenKey] = struct{}{}
	defer delete(seen, seenKey)

	visitVal := VisitType{Descriptor: obj}
	path = append(path, visitVal)
	if fn(visitVal, path) != Descend {
		return
	}

	switch desc := obj.Descriptor.(type) {
	case protoreflect.MessageDescriptor:
		for i := range desc.Fields().Len() {
			field := desc.Fields().Get(i)

			path := path
			if oneof := field.ContainingOneof(); oneof != nil {
				// Place oneof on the path, so that we have a place in the
				// resulting tree to generate the type switch for the oneof
				// with all the implementing types as children.
				visitVal := VisitType{
					Descriptor: oneof,
					FieldName:  snakeToPascalCase(oneof.Name()),
				}
				path = append(path, visitVal)
			}

			visitVal := VisitType{
				Descriptor: field,
				FieldName:  camelToPascalCase(field.JSONName()),
			}
			path = append(path, visitVal)
			switch fn(visitVal, path) {
			case Abort:
				return
			case Prune:
				continue
			}

			if field.IsMap() {
				valueDesc := field.MapValue()
				if valueDesc.Kind() == protoreflect.MessageKind {
					visit(seen, path, VisitType{Descriptor: valueDesc.Message()}, fn)
				}
			} else if field.Kind() == protoreflect.MessageKind {
				msg := field.Message()
				visit(seen, path, VisitType{Descriptor: msg}, fn)
			}
		}
	default:
		panic("visit call requires MessageDescriptor")
	}
}

// camelToPascalCase converts a camelCaseName to a PascalCaseName
//
// Ex:
//   - childWorkflowExecutionFailureInfo -> ChildWorkflowExecutionFailureInfo
//   - activityType -> ActivityType
//
// (This just capitalizes the first letter)
func camelToPascalCase[T ~string](s T) string {
	if len(s) == 0 {
		return string(s)
	}
	result := []rune(s)
	result[0] = unicode.ToUpper(result[0])
	return string(result)
}

// snakeToPascalCase converts a snake_case_name to a PascalCaseName.
//
// Ex:
//   - activity_task_failed_event_attributes -> ActivityTaskFailedEventAttributes
//   - workflow_task_failed_event_attributes -> WorkflowTaskFailedEventAttributes
//   - failure -> Failure
func snakeToPascalCase[T ~string](s T) string {
	if len(s) == 0 {
		return string(s)
	}

	src := []rune(s)
	dest := []rune{}
	// First letter is capitalized
	dest = append(dest, unicode.ToUpper(src[0]))
	i := 1
	for i+1 < len(src) {
		// `_c` --> `C`
		if src[i] == '_' {
			dest = append(dest, unicode.ToUpper(src[i+1]))
			i++
		} else {
			dest = append(dest, src[i])
		}
		i++
	}
	for i < len(src) {
		dest = append(dest, src[i])
		i++
	}
	return string(dest)
}

func (p VisitPath) String() string {
	parts := make([]string, 0, len(p))
	for _, v := range p {
		parts = append(parts, v.GoName())
	}
	return strings.Join(parts, "/")
}
