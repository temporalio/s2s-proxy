package main

import (
	"fmt"
	"io"
	"math/rand/v2"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type (
	Emitter struct {
		Handlers []*Handler
	}

	Handler struct {
		SearchFor  string
		Invocation func(string) string

		matchedPaths []VisitPath
	}
)

func NewEmitter() *Emitter {
	return &Emitter{}
}

func (e *Emitter) AddHandler(match string, invocation func(string) string) {
	e.Handlers = append(e.Handlers, &Handler{
		SearchFor:  match,
		Invocation: invocation,
	})
}

func (e *Emitter) Visit(mt protoreflect.MessageType) bool {
	Visit(mt.Descriptor(), e.visit)
	return true
}

func (e *Emitter) visit(obj VisitType, path VisitPath) {
	for _, handler := range e.Handlers {
		if handler.SearchFor == obj.GoName() {
			pathCopy := make(VisitPath, len(path))
			copy(pathCopy, path) // todo: path is reused during the visitor / changes as it goes.
			handler.matchedPaths = append(handler.matchedPaths, pathCopy)
			fmt.Printf("// emit - %s path=%s\n", obj.Name(), pathCopy)
		}
	}
}

func (e *Emitter) Generate(out io.Writer) {
	e.genPreamble(out)

	fmt.Fprintln(out, "func VisitMessage(vAny any) {")
	fmt.Fprintln(out, "switch root := vAny.(type) {")

	for _, handler := range e.Handlers {
		for _, path := range handler.matchedPaths {
			fmt.Fprintf(out, "// name=%v goname=%v\n", path[0].Name(), path[0].GoName())
			fmt.Fprintf(out, "case *%s:\n", path[0].GoQualifiedName())
			e.emit(out, "root", path[1:], handler)
		}
	}
	fmt.Fprintln(out, "}")
	fmt.Fprintln(out, "}")

}

func (*Emitter) genPreamble(out io.Writer) {
	// TODO: Some way to discover the necessary packages as we visit types.
	fmt.Fprintln(out, `package main_test

	import (
		"go.temporal.io/api/workflowservice/v1"
		"go.temporal.io/api/history/v1"
		serverhistory "go.temporal.io/server/api/history/v1"
		serveradminservice "go.temporal.io/server/api/adminservice/v1"
		serverpersistence "go.temporal.io/server/api/persistence/v1"
		serverreplication "go.temporal.io/server/api/replication/v1"
	)`)
}

func (e *Emitter) emit(out io.Writer, parentVar string, path VisitPath, handler *Handler) {
	if len(path) == 0 {
		fmt.Println(handler.Invocation(parentVar))
		return
	}
	part := path[0]

	switch desc := part.Descriptor.(type) {
	case protoreflect.FieldDescriptor:
		if desc.IsMap() {
			fmt.Printf("for _, val := range %s.%s {\n", parentVar, part.GoGetter())
			e.emit(out, "val", path[1:], handler)
			fmt.Println("}")
		} else if desc.IsList() {
			fmt.Printf("for _, item := range %s.%s {\n", parentVar, part.GoGetter())
			e.emit(out, "item", path[1:], handler)
			fmt.Println("}")
		} else if oneof := desc.ContainingOneof(); oneof != nil {
			fmt.Printf("switch oneof := %s.%s.(type) {\n", parentVar, part.GoGetter())
			fmt.Printf("case *%s.%s:\n", part.GoPackageName(), getOneofWrapperType(path[0], path[1]))
			fmt.Printf("x := oneof.%s\n", path[1].GoName())
			e.emit(out, "x", path[1:], handler)
			fmt.Println("}")
		} else {
			n := rand.Int()
			varName := fmt.Sprintf("y%d", n)
			fmt.Printf("%s := %s.%s\n", varName, parentVar, part.GoGetter())
			e.emit(out, varName, path[1:], handler)
		}
	default:
		e.emit(out, parentVar, path[1:], handler)
	}
}

func (p VisitPath) String() string {
	parts := make([]string, 0, len(p))
	for _, v := range p {
		parts = append(parts, v.GoName())
	}
	return strings.Join(parts, "/")
}

// Return the "wrapper" Golang interface for `oneof` fields.
//
// Protobuf `oneof` fields are generated as an interface:
//
//		type ReplicationTask struct {
//			Attributes isReplicationTask_Attributes `protobuf_oneof:"attributes"`
//	     ...
//		}
//
// The interface is implemented by "wrapper" types which seemingly do not appear
// in the protobuf reflection registry, so we do not enounter these "wrapper"
// type names while visiting the protobuf type hierachy.
//
//	 type ReplicationTask_SyncVersionedTransitionTaskAttributes struct {
//		  SyncVersionedTransitionTaskAttributes *SyncVersionedTransitionTaskAttributes
//	 }
//
// This returns the implementing type, e.g. "ReplicationTask_SyncVersionedTransitionTaskAttributes",
// given the interface field (e.g. `Attributes`) and the wrapped field (e.g. `SyncVersionedTransitionTaskAttributes`)
func getOneofWrapperType(oneof, typ VisitType) string {
	fmt.Printf("// oneof: name=%v goname=%v parent.Name=%v\n", oneof.Name(), oneof.GoName(), oneof.Parent().Name())
	fmt.Printf("// typ  : name=%v goname=%v parent.Name=%v\n", typ.Name(), typ.GoName(), typ.Parent().Name())

	// special cases - these are not entirely consistent
	if strings.Contains(string(oneof.FullName()), "ExecuteMultiOperationResponse") {
		return fmt.Sprintf("ExecuteMultiOperationResponse_Response_%s", snakeToPascalCase(oneof.Name()))
	}
	if strings.Contains(string(oneof.FullName()), "StreamWorkflowReplicationMessagesResponse") {
		return "StreamWorkflowReplicationMessagesResponse_Messages"
	}

	//return string(oneof.Parent().Name()) + "_" + typ.GoName()
	return string(oneof.Parent().Name()) + "_" + snakeToPascalCase(oneof.Name())
}
