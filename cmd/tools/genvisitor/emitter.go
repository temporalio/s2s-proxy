package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	// CurrentVersion means to emit code for the current version of protos.
	CurrentVersion Mode = iota

	// Gogo122Version means to emit code for the older gogo-based protos
	// from Temporal v1.22. The way this works is to walk type hierarchies
	// for current version of protos, but convert the current protos back
	// to the corresponding gogo-based types/packages.
	Gogo122Version
)

const (
	defaultPackageName   = "main_test"
	defaultFuncSignature = "func VisitMessage(vAny any)"

	// localModule is this repository's module path, which gci puts in its own
	// import group after third-party imports.
	localModule = "github.com/temporalio/s2s-proxy"
)

type (
	Emitter struct {
		logger        log.Logger
		mode          Mode
		packageName   string
		funcSignature string
		funcTrailer   string
		handlers      []*Handler
		check         func(VisitType, VisitPath, bool) (Action, error)
		errs          []error
		// used holds the packages the generated code names, which are the only
		// ones it may import: types reached through getters are never named.
		used         map[string]struct{}
		extraImports map[string]struct{}
		root         *Tree
		inScopeVars  map[string]struct{}
		// blockFrees holds, per open Go block, the variable names to release
		// when that block closes.
		blockFrees [][]func()
	}

	importLine struct {
		path string
		text string
	}

	// Handler matches a field in the type hierarchy to a function that generates code.
	Handler struct {
		// Include returns whether to include this path during code generation.
		Include func(VisitType, VisitPath) bool
		// Prune stops the walk descending below a node this handler matched.
		Prune bool
		// Invocation returns a snippet of generated code. It is passed a variable
		// holding the matched value and the path that matched.
		Invocation func(string, VisitPath) string
	}

	Mode int
)

func NewEmitter(logger log.Logger, mode Mode) *Emitter {
	return &Emitter{
		logger:        logger,
		mode:          mode,
		packageName:   defaultPackageName,
		funcSignature: defaultFuncSignature,
		used:          make(map[string]struct{}),
		extraImports:  make(map[string]struct{}),
		root:          NewTree(),
		inScopeVars:   map[string]struct{}{},
	}
}

func (e *Emitter) SetPackageName(name string)        { e.packageName = name }
func (e *Emitter) SetFunctionSignature(sig string)   { e.funcSignature = sig }
func (e *Emitter) SetFunctionTrailer(trailer string) { e.funcTrailer = trailer }

func (e *Emitter) AddHandler(include func(VisitType, VisitPath) bool, prune bool, invocation func(string, VisitPath) string) {
	e.handlers = append(e.handlers, &Handler{
		Include:    include,
		Prune:      prune,
		Invocation: invocation,
	})
}

// SetCheck installs a function offered every node after the handlers, with
// whether any handler matched it. Cycle nodes are offered too, and only to it.
// A non-Descend action overrides the handlers'; an error is collected, the node
// is pruned, and Generate fails.
func (e *Emitter) SetCheck(check func(VisitType, VisitPath, bool) (Action, error)) {
	e.check = check
}

// Err returns every error the check reported, sorted so the output is stable.
func (e *Emitter) Err() error {
	msgs := make([]string, 0, len(e.errs))
	for _, err := range e.errs {
		msgs = append(msgs, err.Error())
	}
	slices.Sort(msgs)

	errs := make([]error, 0, len(msgs))
	for _, m := range msgs {
		errs = append(errs, errors.New(m))
	}
	return errors.Join(errs...)
}

func (e *Emitter) AddImport(s string) {
	e.extraImports[s] = struct{}{}
}

func (e *Emitter) Visit(mt protoreflect.MessageType) {
	Visit(mt.Descriptor(), e.visit)
}

func (e *Emitter) visit(obj VisitType, path VisitPath) Action {
	if obj.Cycle {
		if e.check != nil {
			if _, err := e.check(obj, path, false); err != nil {
				e.errs = append(e.errs, err)
			}
		}
		return Descend
	}
	if e.mode == Gogo122Version && shouldIgnoreTypeIfDoesntExistIn122(obj.Descriptor) {
		return Abort
	}

	e.logger.Debug("Emitter.visit",
		tag.NewStringTag("obj", string(obj.FullName())),
		tag.NewStringTag("path", path.String()),
	)

	action, matched := Descend, false
	for _, handler := range e.handlers {
		if handler.Include(obj, path) {
			matched = true
			pathCopy := make(VisitPath, len(path))
			copy(pathCopy, path) // path is reused during the visitor / changes as it goes.
			e.root.Insert(pathCopy, handler)
			if handler.Prune {
				action = Prune
			}
		}
	}

	if e.check != nil {
		a, err := e.check(obj, path, matched)
		if err != nil {
			e.errs = append(e.errs, err)
			return Prune
		}
		if a != Descend {
			action = a
		}
	}

	return action
}

// Generate writes the generated source to out, or returns Err without writing
// anything when the check reported problems.
func (e *Emitter) Generate(out io.Writer) error {
	if err := e.Err(); err != nil {
		return err
	}

	// The body goes first, into a buffer, because the imports are only known
	// once it has been written.
	var body bytes.Buffer
	writef(&body, "%s {\n", e.funcSignature)
	writeln(&body, "switch root := vAny.(type) {")
	for _, typ := range e.root.SortedTypes() {
		e.used[typ.GoImportPath()] = struct{}{}
		writef(&body, "case *%s:\n", typ.GoQualifiedName())
		if child := e.root.Children[typ.GoName()]; child != nil {
			e.openBlock()
			e.emit(&body, "root", child)
			e.closeBlock()
		}
	}
	writeln(&body, "}")
	writeln(&body, e.funcTrailer)
	writeln(&body, "}")

	e.genPreamble(out)
	_, err := out.Write(body.Bytes())
	return err
}

func (e *Emitter) genPreamble(out io.Writer) {
	writeln(out, `// Code generated by cmd/tools/genvisitor. DO NOT EDIT.`)
	writef(out, "package %s\n", e.packageName)

	// Grouped the way gci groups them (standard, default, localmodule) and sorted
	// by path, so the output is stable without running a formatter over it.
	var groups [3][]importLine
	for imp := range e.used {
		alias := getImportAlias(imp)
		if e.mode == Gogo122Version {
			imp = replaceWith122Import(imp)
		}
		g := importGroup(imp)
		groups[g] = append(groups[g], importLine{path: imp, text: fmt.Sprintf("%s %q", alias, imp)})
	}
	for imp := range e.extraImports {
		g := importGroup(imp)
		groups[g] = append(groups[g], importLine{path: imp, text: fmt.Sprintf("%q", imp)})
	}

	writeln(out, "import (")
	first := true
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		slices.SortFunc(group, func(a, b importLine) int { return strings.Compare(a.path, b.path) })
		if !first {
			writeln(out)
		}
		first = false
		for _, l := range group {
			writeln(out, l.text)
		}
	}
	writeln(out, ")")
}

// importGroup returns 0 for the standard library, 2 for this module, and 1 for
// everything else.
func importGroup(path string) int {
	switch {
	case !strings.Contains(strings.SplitN(path, "/", 2)[0], "."):
		return 0
	case strings.HasPrefix(path, localModule):
		return 2
	default:
		return 1
	}
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
				e.openBlock()
				e.emit(out, varName, node.Children[vt.GoName()])
				e.closeBlock()
				writeln(out, "}")
			} else if desc.IsList() {
				varName, freeVar := e.makeVar("item")
				defer freeVar()
				writef(out, "for _, %s := range %s.%s {\n", varName, parentVar, vt.GoGetter())
				e.openBlock()
				e.emit(out, varName, node.Children[vt.GoName()])
				e.closeBlock()
				writeln(out, "}")
			} else {
				// Declared in the enclosing block, so the name stays taken until
				// that block closes, not just until this subtree is written.
				varName, freeVar := e.makeVar("y")
				e.releaseWithBlock(freeVar)
				writef(out, "%s := %s.%s\n", varName, parentVar, vt.GoGetter())
				e.emit(out, varName, node.Children[vt.GoName()])
			}
		case protoreflect.OneofDescriptor:
			writef(out, "switch oneof := %s.%s.(type) {\n", parentVar, vt.GoGetter())
			e.emitOneOfCases(out, "oneof", vt, node.Children[vt.GoName()])
			writeln(out, "}")
		default:
			e.emit(out, parentVar, node.Children[vt.GoName()])
		}
	}

	for _, h := range node.Handlers {
		writeln(out, h.handler.Invocation(parentVar, h.path))
	}
}

func (e *Emitter) emitOneOfCases(out io.Writer, parentVar string, oneof VisitType, node *Tree) {
	e.used[oneof.GoImportPath()] = struct{}{}
	for _, vt := range node.SortedTypes() {
		writef(out, "case *%s.%s:\n", oneof.GoPackageName(), getOneofWrapperType(oneof, vt))
		varName, freeVar := e.makeVar("x")
		name := vt.GoName()
		writef(out, "%s := %s.%s\n", varName, parentVar, name)
		e.openBlock()
		e.emit(out, varName, node.Children[vt.GoName()])
		e.closeBlock()
		freeVar()
	}
}

func (e *Emitter) makeVar(name string) (string, func()) {
	i := 0
	for {
		i++
		name := fmt.Sprintf("%s%d", name, i)
		if _, ok := e.inScopeVars[name]; !ok {
			e.inScopeVars[name] = struct{}{}
			return name, func() { e.freeVar(name) }
		}
	}
}

func (e *Emitter) openBlock() {
	e.blockFrees = append(e.blockFrees, nil)
}

func (e *Emitter) closeBlock() {
	last := len(e.blockFrees) - 1
	for _, free := range e.blockFrees[last] {
		free()
	}
	e.blockFrees = e.blockFrees[:last]
}

// releaseWithBlock frees a variable name when the innermost open block closes.
func (e *Emitter) releaseWithBlock(free func()) {
	last := len(e.blockFrees) - 1
	e.blockFrees[last] = append(e.blockFrees[last], free)
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
	return goIdent(oneof.Parent()) + "_" + snakeToPascalCase(typ.Name())
}
