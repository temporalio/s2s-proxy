package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// VisitType represents a visited protobuf type.
type VisitType struct {
	FieldName string
	protoreflect.Descriptor
}

func (v VisitType) GoFieldName() string {
	return v.FieldName
}

func (v VisitType) GoName() string {
	if v.FieldName != "" {
		return v.FieldName
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
