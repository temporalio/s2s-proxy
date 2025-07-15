package main

import (
	"fmt"
	"strings"
	"unicode"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type (
	visitor func(VisitType, VisitPath) bool

	VisitPath []VisitType
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
		return
	}
	seen[seenKey] = struct{}{}
	defer delete(seen, seenKey)

	visitVal := VisitType{Descriptor: obj}
	path = append(path, visitVal)
	if !fn(visitVal, path) {
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
			if !fn(visitVal, path) {
				return
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

func camelToPascalCase[T ~string](s T) string {
	if len(s) == 0 {
		return string(s)
	}
	result := []rune(s)
	result[0] = unicode.ToUpper(result[0])
	return string(result)
}

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
