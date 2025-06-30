package main

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// VisitType represents a visited protobuf type.
type VisitType struct {
	GoFieldName string
	protoreflect.Descriptor
}

func (v VisitType) GoName() string {
	if v.GoFieldName != "" {
		return v.GoFieldName
	}
	return string(v.Descriptor.Name())
}

func (v VisitType) GoGetter() string {
	return fmt.Sprintf("Get%s()", v.GoName())
}

func (v VisitType) GoQualifiedName() string {
	return fmt.Sprintf("%s.%s", v.GoPackageName(), v.GoName())
}

func (v VisitType) GoPackageName() string {
	pkg := v.ParentFile().Package()
	isServerPkg := strings.Contains(string(pkg), "temporal.server.api")

	//fmt.Printf("// v = %v\n", v)
	//fmt.Printf("// pkg = %v\n", pkg)
	if pkg.Name() == "v1" {
		//fmt.Printf("// pkg = %v\n", pkg)
		pkg = pkg.Parent()
	}
	result := string(pkg.Name())
	if isServerPkg {
		return "server" + result
	}
	return result
}

func (v VisitType) GoImportPath() string {
	imp := string(v.ParentFile().Package())
	imp = strings.ReplaceAll(imp, ".", "/")
	imp = strings.Replace(imp, "temporal/", "go.temporal.io/", 1)
	return imp

}
