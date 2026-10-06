package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// VisitType represents a visited protobuf type.
type VisitType struct {
	FieldName string
	// Cycle marks a message that already appears earlier on the path.
	Cycle bool
	protoreflect.Descriptor
}

// AsMessage returns the message descriptor behind v, unwrapping the VisitType
// that Visit wraps message nodes in.
func (v VisitType) AsMessage() (protoreflect.MessageDescriptor, bool) {
	switch d := v.Descriptor.(type) {
	case VisitType:
		return d.AsMessage()
	case protoreflect.MessageDescriptor:
		return d, true
	default:
		return nil, false
	}
}

// AsField returns the field descriptor behind v.
func (v VisitType) AsField() (protoreflect.FieldDescriptor, bool) {
	switch d := v.Descriptor.(type) {
	case VisitType:
		return d.AsField()
	case protoreflect.FieldDescriptor:
		return d, true
	default:
		return nil, false
	}
}

// goIdent returns the Go identifier protoc-gen-go uses for d: its name relative
// to the package, with nesting separated by underscores.
func goIdent(d protoreflect.Descriptor) string {
	rel := strings.TrimPrefix(string(d.FullName()), string(d.ParentFile().Package())+".")
	return strings.ReplaceAll(rel, ".", "_")
}

func (v VisitType) GoFieldName() string {
	return v.FieldName
}

func (v VisitType) GoName() string {
	if v.FieldName != "" {
		return v.FieldName
	}
	// Nested messages share short names (AddTasksRequest.Task and Task), so
	// messages go by their full Go identifier.
	if _, ok := v.AsMessage(); ok {
		return goIdent(v)
	}
	return string(v.Name())
}

func (v VisitType) GoTypeName() string {
	return string(v.Name())
}

func (v VisitType) GoGetter() string {
	return fmt.Sprintf("Get%s()", v.FieldName)
}

func (v VisitType) GoQualifiedName() string {
	return fmt.Sprintf("%s.%s", v.GoPackageName(), v.GoName())
}

func (v VisitType) GoPackageName() string {
	return getImportAlias(v.GoImportPath())
}

func (v VisitType) GoImportPath() string {
	imp := string(v.ParentFile().Package())
	imp = strings.ReplaceAll(imp, ".", "/")
	imp = strings.Replace(imp, "temporal/", "go.temporal.io/", 1)
	return imp

}
