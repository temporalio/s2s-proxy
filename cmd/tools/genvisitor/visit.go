package main

import (
	"unicode"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type (
	visitor func(VisitType, VisitPath)

	VisitPath []VisitType
)

func Visit(obj protoreflect.MessageDescriptor, fn visitor) {
	seen := make(map[string]struct{})
	visit(seen, nil, obj, fn)
}

func visit(
	seen map[string]struct{},
	path []VisitType,
	obj protoreflect.MessageDescriptor,
	fn visitor,
) {
	// Key by `Type.Field` so that we visit every field once.
	//
	// Does this handle field names correctly? (Idk)
	seenKey := string(obj.Parent().Name() + "." + obj.Name())
	if _, ok := seen[seenKey]; ok {
		// fmt.Printf("// skipping already seen key %s\n", seenKey)
		return
	}
	seen[seenKey] = struct{}{}

	visitVal := VisitType{Descriptor: obj}
	path = append(path, visitVal)
	fn(visitVal, path)

	for i := range obj.Fields().Len() {
		field := obj.Fields().Get(i)

		// Problem: How do I get the Golang name of this field.
		// Workaround: Convert the json name (`shardMessages`) to the Golang name (`ShardMessages`)
		name := camelToPascalCase(field.JSONName())
		if oneof := field.ContainingOneof(); oneof != nil {
			name = snakeToPascalCase(oneof.Name())
		}

		visitVal := VisitType{
			Descriptor:  field,
			GoFieldName: name,
		}
		path := append(path, visitVal)
		fn(visitVal, path)

		if field.IsMap() {
			valueDesc := field.MapValue()
			if valueDesc.Kind() == protoreflect.MessageKind {
				visit(seen, path, valueDesc.Message(), fn)
			}
		} else if field.Kind() == protoreflect.MessageKind {
			msg := field.Message()
			visit(seen, path, msg, fn)
		} else {
		}
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
