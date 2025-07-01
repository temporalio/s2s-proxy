package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	CurrentVersion Mode = iota
	Gogo122Version
)

type (
	Emitter struct {
		mode          Mode
		funcSignature string
		handlers      []*Handler
		imports       map[string]struct{}
		extraImports  map[string]struct{}
		root          *Tree
		inScopeVars   map[string]struct{}
		trailer       string
	}

	Tree struct {
		Children   map[string]*Tree
		VisitTypes map[string]VisitType
		Items      []*Handler
	}

	Handler struct {
		Include    func(string) bool
		Invocation func(string) string
	}

	Mode int
)

func NewEmitter(mode Mode) *Emitter {
	return &Emitter{
		mode:         mode,
		imports:      make(map[string]struct{}),
		extraImports: make(map[string]struct{}),
		root:         NewTree(),
		inScopeVars:  map[string]struct{}{},
	}
}

func (e *Emitter) SetFunctionSignature(sig string) {
	e.funcSignature = sig
}

func (e *Emitter) SetTrailer(trailer string) {
	e.trailer = trailer
}

func (e *Emitter) AddHandler(include func(string) bool, invocation func(string) string) {
	e.handlers = append(e.handlers, &Handler{
		Include:    include,
		Invocation: invocation,
	})
}

func (e *Emitter) AddImport(s string) {
	e.extraImports[s] = struct{}{}
}

func (e *Emitter) Visit(mt protoreflect.MessageType) {
	if e.mode == Gogo122Version &&
		shouldIgnoreTypeIfDoesntExistIn122(string(mt.Descriptor().FullName())) {
		return
	}
	if mt.Descriptor().FullName() == "temporal.api.workflowservice.v1.ExecuteMultiOperationResponse.Response" {
		// Ignore this nested type. We handle the parent type.
		return
	}
	Visit(mt.Descriptor(), e.visit)
}

func (e *Emitter) visit(obj VisitType, path VisitPath) {
	for _, handler := range e.handlers {
		if handler.Include(obj.GoName()) {
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

	if e.funcSignature != "" {
		writeln(out, e.funcSignature+" {")
	} else {
		writeln(out, "func VisitMessage(vAny any) {")
	}
	writeln(out, "switch root := vAny.(type) {")
	for _, typ := range e.root.SortedTypes() {
		writef(out, "case *%s:\n", typ.GoQualifiedName())
		if child := e.root.Children[typ.GoName()]; child != nil {
			e.emit(out, "root", child)
		}
	}
	writeln(out, "}")

	writeln(out, e.trailer)
	writeln(out, "}")

}

func writeln(out io.Writer, args ...any) {
	_, err := fmt.Fprintln(out, args...)
	if err != nil {
		panic(err)
	}
}

func writef(out io.Writer, msg string, args ...any) {
	_, err := fmt.Fprintf(out, msg, args...)
	if err != nil {
		panic(err)
	}
}

func (e *Emitter) genPreamble(out io.Writer) {
	writeln(out, `package main_test

	import (`)

	for imp := range e.imports {
		alias := ""
		// Alias all server imports as "server<package>"
		// to avoid conflicts with api packages.
		if strings.HasPrefix(imp, "go.temporal.io/server") {
			alias = strings.ReplaceAll(imp, "go.temporal.io/server/api", "")
			alias = strings.ReplaceAll(alias, "v1", "")
			alias = strings.ReplaceAll(alias, "/", "")
			alias = "server" + alias
		}
		if e.mode == Gogo122Version {
			imp = replaceWith122Import(imp)
		}
		writef(out, "%s \"%s\"\n", alias, imp)
	}

	for imp := range e.extraImports {
		writef(out, "\"%s\"\n", imp)

	}

	writeln(out, `)`)
}

func (e *Emitter) emit(out io.Writer, parentVar string, node *Tree) {
	if node == nil {
		return
	}

	for _, vt := range node.SortedTypes() {
		switch desc := vt.Descriptor.(type) {
		case protoreflect.FieldDescriptor:
			if desc.IsMap() {
				varName, freeVar := e.makeVar("val")
				defer freeVar()
				writef(out, "for _, %s := range %s.%s {\n", varName, parentVar, vt.GoGetter())
				e.emit(out, varName, node.Children[vt.GoName()])
				writeln(out, "}")
			} else if desc.IsList() {
				varName, freeVar := e.makeVar("item")
				defer freeVar()
				writef(out, "for _, %s := range %s.%s {\n", varName, parentVar, vt.GoGetter())
				e.emit(out, varName, node.Children[vt.GoName()])
				writeln(out, "}")
			} else if oneof := desc.ContainingOneof(); oneof != nil {
				writef(out, "switch oneof := %s.%s.(type) {\n", parentVar, vt.GoGetter())
				e.emitOneOfCases(out, "oneof", vt, node.Children[vt.GoName()])
				writeln(out, "}")
			} else {
				varName, freeVar := e.makeVar("y")
				defer freeVar()
				writef(out, "%s := %s.%s\n", varName, parentVar, vt.GoGetter())
				e.emit(out, varName, node.Children[vt.GoName()])
			}
		default:
			e.emit(out, parentVar, node.Children[vt.GoName()])
		}
	}

	for _, handler := range node.Items {
		writeln(out, handler.Invocation(parentVar))
	}
}

func (e *Emitter) emitOneOfCases(out io.Writer, parentVar string, oneof VisitType, node *Tree) {
	if node == nil {
		return
	}

	for _, vt := range node.SortedTypes() {
		writef(out, "case *%s.%s:\n", oneof.GoPackageName(), getOneofWrapperType(oneof, vt))
		varName, freeVar := e.makeVar("x")
		name := vt.GoName()
		if name == "WorkflowReplicationMessages" {
			name = "Messages" // workaround
		}

		writef(out, "%s := oneof.%s\n", varName, name)
		e.emit(out, varName, node.Children[vt.GoName()])
		freeVar()
	}

}

func (p VisitPath) String() string {
	parts := make([]string, 0, len(p))
	for _, v := range p {
		parts = append(parts, v.GoName())
	}
	return strings.Join(parts, "/")
}

func (e *Emitter) makeVar(name string) (string, func()) {
	for i := 1; i < 50; i++ {
		name := fmt.Sprintf("%s%d", name, i)
		if _, ok := e.inScopeVars[name]; !ok {
			e.inScopeVars[name] = struct{}{}
			return name, func() { e.freeVar(name) }
		}
	}
	panic("failed to generate unique variable name")
}

func (e *Emitter) freeVar(name string) {
	delete(e.inScopeVars, name)
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
func getOneofWrapperType(oneof, typ VisitType) string {
	// special cases - these are not entirely consistent
	if strings.Contains(string(oneof.FullName()), "ExecuteMultiOperationResponse") {
		return fmt.Sprintf("ExecuteMultiOperationResponse_Response_%s", snakeToPascalCase(oneof.Name()))
	}
	if strings.Contains(string(oneof.FullName()), "StreamWorkflowReplicationMessagesResponse") {
		return "StreamWorkflowReplicationMessagesResponse_Messages"
	}

	//return string(oneof.Parent().Name()) + "_" + typ.GoName()
	return string(oneof.Parent().Name()) + "_" + snakeToPascalCase(typ.Name())
}

func NewTree() *Tree {
	return &Tree{
		Children:   map[string]*Tree{},
		VisitTypes: map[string]VisitType{},
	}
}

func (t *Tree) SortedTypes() []VisitType {
	result := make([]VisitType, 0, len(t.VisitTypes))
	for _, typ := range t.VisitTypes {
		result = append(result, typ)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].FullName() < result[j].FullName()
	})
	return result
}
