package main

import (
	"io"
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// Tree is used to store matched paths in a type hierarchy.
type Tree struct {
	// Types contains the types at this node in the tree.
	Types map[string]VisitType
	// Children contains children of any of the Types (keyed by same key as Types)
	Children map[string]*Tree
	// Handlers are used by the Emitter for code generation.
	Handlers []*Handler
}

func NewTree() *Tree {
	return &Tree{
		Children: map[string]*Tree{},
		Types:    map[string]VisitType{},
	}
}

func (t *Tree) Insert(path VisitPath, handler *Handler) {
	current := t
	for _, p := range path {
		current.Types[p.GoName()] = p

		if _, ok := current.Children[p.GoName()]; !ok {
			current.Children[p.GoName()] = NewTree()
		}
		current = current.Children[p.GoName()]
	}
	current.Handlers = append(current.Handlers, handler)
}

func (t *Tree) SortedTypes() []VisitType {
	result := make([]VisitType, 0, len(t.Types))
	for _, typ := range t.Types {
		result = append(result, typ)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].FullName() < result[j].FullName()
	})
	return result
}

// Dump prints the tree to the given writer for debugging.
func (t *Tree) Dump(out io.Writer) {
	t.dump(out, 0)
}

func (t *Tree) dump(out io.Writer, depth int) {
	spaces := make([]rune, 0, depth)
	for range depth {
		spaces = append(spaces, ' ', ' ')
	}
	indent := string(spaces)

	for _, h := range t.Handlers {
		writef(out, "%s%s\n", indent, h.Invocation("v"))
	}

	for f, ch := range t.Children {
		vt := t.Types[f]
		_, isoneof := vt.Descriptor.(protoreflect.OneofDescriptor)
		write(out, indent)
		if isoneof {
			writef(out, " oneof")
		}
		writef(out, " %s\n", vt.FullName())
		ch.dump(out, depth+1)
	}
}
