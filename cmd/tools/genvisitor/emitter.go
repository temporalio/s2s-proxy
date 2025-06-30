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
		handlers []*Handler
		imports  map[string]struct{}
		root     *Tree
	}

	Tree struct {
		Children   map[string]*Tree
		VisitTypes map[string]VisitType
		Items      []*Handler
	}

	Handler struct {
		SearchFor  string
		Invocation func(string) string
	}
)

func NewEmitter() *Emitter {
	return &Emitter{
		imports: make(map[string]struct{}),
		root:    NewTree(),
	}
}

func NewTree() *Tree {
	return &Tree{
		Children:   map[string]*Tree{},
		VisitTypes: map[string]VisitType{},
	}

}

func (e *Emitter) AddHandler(match string, invocation func(string) string) {
	e.handlers = append(e.handlers, &Handler{
		SearchFor:  match,
		Invocation: invocation,
	})
}

func (e *Emitter) Visit(mt protoreflect.MessageType) bool {
	if mt.Descriptor().FullName() == "temporal.api.workflowservice.v1.ExecuteMultiOperationResponse.Response" {
		// Ignore this nested type. We handle the parent type.
		return true
	}
	Visit(mt.Descriptor(), e.visit)
	return true
}

func (e *Emitter) visit(obj VisitType, path VisitPath) {
	for _, handler := range e.handlers {
		if handler.SearchFor == obj.GoName() {
			pathCopy := make(VisitPath, len(path))
			copy(pathCopy, path) // todo: path is reused during the visitor / changes as it goes.
			e.insert(pathCopy, handler)
			e.discoverImports(pathCopy)
		}
	}
}

func (e *Emitter) insert(path VisitPath, handler *Handler) {
	current := e.root
	for _, p := range path {
		current.VisitTypes[p.GoName()] = p

		if _, ok := current.Children[p.GoName()]; !ok {
			current.Children[p.GoName()] = NewTree()
		}
		current = current.Children[p.GoName()]
	}
	current.Items = append(current.Items, handler)
}

func (e *Emitter) discoverImports(path VisitPath) {
	for _, obj := range path {
		e.imports[obj.GoImportPath()] = struct{}{}
	}
}

func (e *Emitter) Generate(out io.Writer) {
	e.genPreamble(out)

	fmt.Fprintln(out, "func VisitMessage(vAny any) {")
	fmt.Fprintln(out, "switch root := vAny.(type) {")
	for _, typ := range e.root.VisitTypes {
		fmt.Printf("case *%s:", typ.GoQualifiedName())
		if child := e.root.Children[typ.GoName()]; child != nil {
			e.emit(out, "root", child)
		}
	}
	fmt.Fprintln(out, "}")
	fmt.Fprintln(out, "}")

}

func (e *Emitter) genPreamble(out io.Writer) {
	fmt.Fprintln(out, `package main_test

	import (`)

	for imp := range e.imports {
		alias := ""
		if strings.HasPrefix(imp, "go.temporal.io/server") {
			alias = strings.ReplaceAll(imp, "go.temporal.io/server/api", "")
			alias = strings.ReplaceAll(alias, "v1", "")
			alias = strings.ReplaceAll(alias, "/", "")
			alias = "server" + alias
		}
		fmt.Fprintf(out, "%s \"%s\"\n", alias, imp)
	}
	fmt.Fprintln(out, `)`)
}

func (e *Emitter) emit(out io.Writer, parentVar string, node *Tree) {
	if node == nil {
		return
	}

	for _, vt := range node.VisitTypes {
		switch desc := vt.Descriptor.(type) {
		case protoreflect.FieldDescriptor:
			if desc.IsMap() {
				fmt.Printf("for _, val := range %s.%s {\n", parentVar, vt.GoGetter())
				if child := node.Children[vt.GoName()]; child != nil {
					e.emit(out, "val", child)
				}
				fmt.Println("}")
			} else if desc.IsList() {
				fmt.Printf("for _, item := range %s.%s {\n", parentVar, vt.GoGetter())
				if child := node.Children[vt.GoName()]; child != nil {
					e.emit(out, "item", child)
				}
				fmt.Println("}")
			} else if oneof := desc.ContainingOneof(); oneof != nil {
				fmt.Printf("switch oneof := %s.%s.(type) {\n", parentVar, vt.GoGetter())
				fmt.Printf("case *%s.%s:\n", vt.GoPackageName(), getOneofWrapperType(vt))
				fmt.Printf("x := oneof.%s\n", snakeToPascalCase(vt.Name()))

				if child := node.Children[vt.GoName()]; child != nil {
					e.emit(out, "x", child)
				}

				fmt.Println("}")
			} else {
				n := rand.Int()
				varName := fmt.Sprintf("y%d", n)
				fmt.Printf("%s := %s.%s\n", varName, parentVar, vt.GoGetter())

				if child := node.Children[vt.GoName()]; child != nil {
					e.emit(out, varName, child)
				}
			}
		default:
			if child := node.Children[vt.GoName()]; child != nil {
				e.emit(out, parentVar, child)
			}
		}
	}

	for _, handler := range node.Items {
		fmt.Fprintln(out, handler.Invocation(parentVar))
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
//	        ...
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
func getOneofWrapperType(oneof VisitType) string {
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
