package interceptor

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"

	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/server/api/persistence/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

const (
	payloadFullName  protoreflect.FullName = "temporal.api.common.v1.Payload"
	dataBlobFullName protoreflect.FullName = "temporal.api.common.v1.DataBlob"
	anyFullName      protoreflect.FullName = "google.protobuf.Any"
	publicAPIPrefix                        = "temporal.api."
	wellKnownPrefix                        = "google.protobuf."
)

// CHASM state is the server's component tree: scheduler, Nexus operation, and
// standalone activity state, among others. Each node is serialized into its own
// DataBlob, and those blobs hold customer payloads. The proxy does not link the
// server's CHASM packages, which pull in most of the server, so
// cmd/tools/genvisitor reads the server's CHASM registry and writes what the
// proxy needs into chasm_schema_gen.go: the type behind every component, data
// node, and task, and the descriptors of the messages they decode to.

type (
	// chasmComponentSchema is a registered CHASM component.
	chasmComponentSchema struct {
		fqn string
		// state is the message the component's own node holds, if any.
		state protoreflect.FullName
		// children are the component's child nodes, by node name.
		children map[string]chasmChildSchema
	}

	// chasmChildSchema is a child node of a component: another component or
	// a data node, alone or in a collection keyed by the next path segment.
	chasmChildSchema struct {
		collection bool
		component  uint32
		data       protoreflect.FullName
	}

	// chasmTaskSchema is a registered CHASM task. data is empty for a task
	// that carries none.
	chasmTaskSchema struct {
		fqn  string
		data protoreflect.FullName
	}

	// chasmResolver resolves the embedded CHASM files first, then the public
	// and well-known ones the proxy links.
	chasmResolver struct {
		local *protoregistry.Files
	}
)

// chasmFiles holds the embedded CHASM descriptors, built once.
var chasmFiles = sync.OnceValues(loadChasmFiles)

func loadChasmFiles() (*protoregistry.Files, error) {
	var set descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal([]byte(chasmDescriptorSet), &set); err != nil {
		return nil, fmt.Errorf("failed to read CHASM descriptors: %w", err)
	}

	files := new(protoregistry.Files)
	resolver := chasmResolver{local: files}
	for _, fdp := range set.GetFile() {
		fd, err := protodesc.NewFile(fdp, resolver)
		if err != nil {
			return nil, fmt.Errorf("failed to build CHASM descriptor %s: %w", fdp.GetName(), err)
		}
		if err := files.RegisterFile(fd); err != nil {
			return nil, fmt.Errorf("failed to register CHASM descriptor %s: %w", fdp.GetName(), err)
		}
	}

	return files, nil
}

func (r chasmResolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	if fd, err := r.local.FindFileByPath(path); err == nil {
		return fd, nil
	}

	return protoregistry.GlobalFiles.FindFileByPath(path)
}

func (r chasmResolver) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	if d, err := r.local.FindDescriptorByName(name); err == nil {
		return d, nil
	}

	return protoregistry.GlobalFiles.FindDescriptorByName(name)
}

// chasmMessage returns the descriptor of name, an embedded CHASM message or a
// public one the proxy links.
func chasmMessage(name protoreflect.FullName) (protoreflect.MessageDescriptor, error) {
	files, err := chasmFiles()
	if err != nil {
		return nil, err
	}

	d, err := chasmResolver{local: files}.FindDescriptorByName(name)
	if err != nil {
		return nil, fmt.Errorf("unknown CHASM message %s: %w", name, err)
	}
	md, ok := d.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("CHASM type %s is not a message", name)
	}

	return md, nil
}

// chasmDataType walks the schema from the component with type ID root along
// path, the names of the nodes below it, and returns the message of the data
// node path ends at. It reports false when path leaves the schema, or ends at
// anything but a data node.
func chasmDataType(root uint32, path []string) (protoreflect.FullName, bool) {
	comp, ok := chasmComponents[root]
	for i := 0; ok && i < len(path); {
		child, found := comp.children[path[i]]
		if !found {
			return "", false
		}
		i++
		if child.collection {
			// The collection node itself, or the key of one of its items.
			if i == len(path) {
				return "", false
			}
			i++
		}
		if i == len(path) {
			return child.data, child.data != ""
		}
		if child.component == 0 {
			// A data node has no children.
			return "", false
		}
		comp, ok = chasmComponents[child.component]
	}

	return "", false
}

// chasmTree indexes the nodes of one map of CHASM nodes by their decoded path,
// so that a data node can be typed from a component above it.
type chasmTree struct {
	nodes map[string]*persistence.ChasmNode
}

func newChasmTree(nodes map[string]*persistence.ChasmNode) (chasmTree, error) {
	t := chasmTree{nodes: make(map[string]*persistence.ChasmNode, len(nodes))}
	for key, node := range nodes {
		path, err := decodeChasmPath(key)
		if err != nil {
			return chasmTree{}, err
		}
		t.nodes[joinChasmPath(path)] = node
	}

	return t, nil
}

// dataType returns the message of the data node at path. It walks the schema
// from the first component it knows the type of: the archetype, the root of
// the execution, when not zero, and otherwise the nearest component node
// above path in the tree, the root included.
func (t chasmTree) dataType(archetype uint32, path []string) (protoreflect.FullName, bool) {
	if archetype != 0 {
		if name, ok := chasmDataType(archetype, path); ok {
			return name, true
		}
	}

	for i := len(path) - 1; i >= 0; i-- {
		node, ok := t.nodes[joinChasmPath(path[:i])]
		if !ok {
			continue
		}
		attrs := node.GetMetadata().GetComponentAttributes()
		if attrs == nil {
			continue
		}

		return chasmDataType(attrs.GetTypeId(), path[i:])
	}

	return "", false
}

// joinChasmPath joins decoded path segments into a key no two paths share.
func joinChasmPath(path []string) string {
	return strings.Join(path, "\x00")
}

// decodeChasmPath splits a CHASM node key into node names, undoing the
// server's escaping. It follows the server's defaultPathEncoder: names are
// separated by '$', or by '#' before an item of a collection, and '\' escapes
// the next character. The root is the empty key.
func decodeChasmPath(key string) ([]string, error) {
	if key == "" {
		return nil, nil
	}

	var (
		path    []string
		b       strings.Builder
		escaped bool
	)
	for _, r := range key {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == '$' || r == '#':
			path = append(path, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if escaped {
		return nil, fmt.Errorf("CHASM node path %q ends with an escape character", key)
	}

	return append(path, b.String()), nil
}

// archetypeIDKey is where the generated visitor leaves the archetype ID of the
// execution the part of an admin message being visited belongs to.
type archetypeIDKey struct{}

// withArchetypeID returns ctx carrying id, the type ID of the root component of
// the execution whose CHASM nodes are about to be visited.
func withArchetypeID(ctx context.Context, id uint32) context.Context {
	return context.WithValue(ctx, archetypeIDKey{}, id)
}

// archetypeIDFrom returns the archetype ID on ctx, or zero for none.
func archetypeIDFrom(ctx context.Context) uint32 {
	id, _ := ctx.Value(archetypeIDKey{}).(uint32)
	return id
}

// visitChasmNodes visits the payloads in a map of CHASM nodes, keyed by node
// path: the state of every component, the data nodes, and the data of every
// task. A node or task whose type is unknown is reported to opts.Opaque whole.
// It takes the whole map because a data node is typed by where it sits.
func visitChasmNodes(ctx context.Context, opts AdminVisitOptions, path string, nodes map[string]*persistence.ChasmNode) error {
	if len(nodes) == 0 {
		return nil
	}

	tree, err := newChasmTree(nodes)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	archetype := archetypeIDFrom(ctx)

	// Sorted, so a failure always names the same node.
	for _, key := range slices.Sorted(maps.Keys(nodes)) {
		node := nodes[key]
		nodePath := fmt.Sprintf("%s[%s]", path, key)

		var (
			name  protoreflect.FullName
			typed bool
		)
		switch attrs := node.GetMetadata().GetAttributes().(type) {
		case *persistence.ChasmNodeMetadata_ComponentAttributes:
			ca := attrs.ComponentAttributes
			comp, ok := chasmComponents[ca.GetTypeId()]
			name, typed = comp.state, ok && comp.state != ""

			for i, task := range ca.GetSideEffectTasks() {
				if err := visitChasmTask(ctx, opts, fmt.Sprintf("%s/SideEffectTasks[%d]/Data", nodePath, i), task.GetTypeId(), task.GetData()); err != nil {
					return err
				}
			}
			for i, task := range ca.GetPureTasks() {
				if err := visitChasmTask(ctx, opts, fmt.Sprintf("%s/PureTasks[%d]/Data", nodePath, i), task.GetTypeId(), task.GetData()); err != nil {
					return err
				}
			}
			if err := visitMessage(ctx, opts, ca.GetUserMetadata()); err != nil {
				return err
			}
		case *persistence.ChasmNodeMetadata_DataAttributes:
			segments, _ := decodeChasmPath(key)
			name, typed = tree.dataType(archetype, segments)
		}

		dataPath := nodePath + "/Data"
		if !typed {
			if err := visitOpaque(opts, dataPath, node.GetData().GetData()); err != nil {
				return err
			}
			continue
		}
		if err := visitChasmBlob(ctx, opts, dataPath, name, node.GetData()); err != nil {
			return err
		}
	}

	return nil
}

// visitChasmTaskInfo visits the payloads in the data of a CHASM task, as a
// history task carries it.
func visitChasmTaskInfo(ctx context.Context, opts AdminVisitOptions, path string, info *persistence.ChasmTaskInfo) error {
	if info == nil {
		return nil
	}

	return visitChasmTask(ctx, opts, path+"/Data", info.GetTypeId(), info.GetData())
}

// visitChasmTask visits the payloads in the data of a task of type typeID.
func visitChasmTask(ctx context.Context, opts AdminVisitOptions, path string, typeID uint32, blob *common.DataBlob) error {
	task, ok := chasmTasks[typeID]
	if !ok || task.data == "" {
		return visitOpaque(opts, path, blob.GetData())
	}

	return visitChasmBlob(ctx, opts, path, task.data, blob)
}

// visitChasmBlob decodes blob as a message called name, visits the payloads in
// it, and writes it back. A blob that is not proto3 or does not decode as name
// is reported to opts.Opaque whole; parts of a decoded one that cannot be seen
// into are reported on their own. Like visitEventBlob, it always reserializes.
func visitChasmBlob(ctx context.Context, opts AdminVisitOptions, path string, name protoreflect.FullName, blob *common.DataBlob) error {
	if len(blob.GetData()) == 0 {
		return nil
	}
	if blob.GetEncodingType() != enums.ENCODING_TYPE_PROTO3 {
		return visitOpaque(opts, path, blob.GetData())
	}

	md, err := chasmMessage(name)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	m := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(blob.GetData(), m); err != nil {
		return visitOpaque(opts, path, blob.GetData())
	}

	if err := visitChasmMessage(ctx, opts, path, nil, m); err != nil {
		return err
	}

	out, err := proto.MarshalOptions{Deterministic: true}.Marshal(m)
	if err != nil {
		return fmt.Errorf("%s: failed to serialize CHASM data: %w", path, err)
	}
	blob.Data = out

	return nil
}

// visitChasmMessage visits the payloads in m, decoded CHASM data held by field
// fd, nil for the root, and updates m in place. A public message goes to the
// payload visitor whole; a server-internal one is walked field by field.
func visitChasmMessage(ctx context.Context, opts AdminVisitOptions, path string, fd protoreflect.FieldDescriptor, m protoreflect.Message) error {
	name := m.Descriptor().FullName()
	switch {
	case name == anyFullName:
		return visitChasmAny(ctx, opts, path, fd, m)
	case name == dataBlobFullName:
		if fd != nil && chasmIgnoredFields[fd.FullName()] {
			return nil
		}
		b, err := proto.Marshal(m.Interface())
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		return visitOpaque(opts, path, b)
	case strings.HasPrefix(string(name), wellKnownPrefix):
		return nil
	case strings.HasPrefix(string(name), publicAPIPrefix):
		return visitChasmPublic(ctx, opts, path, fd, m)
	}

	if err := visitOpaque(opts, path+"/<unknown fields>", m.GetUnknown()); err != nil {
		return err
	}

	// Collected first: changing a message while ranging over it is undefined.
	type field struct {
		fd protoreflect.FieldDescriptor
		v  protoreflect.Value
	}
	var fields []field
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		fields = append(fields, field{fd, v})
		return true
	})

	for _, f := range fields {
		fieldPath := path + "/" + string(f.fd.Name())
		switch {
		case f.fd.IsMap():
			mv := f.fd.MapValue()
			var err error
			f.v.Map().Range(func(k protoreflect.MapKey, v protoreflect.Value) bool {
				err = visitChasmValue(ctx, opts, fmt.Sprintf("%s[%v]", fieldPath, k.Interface()), f.fd, mv, v)
				return err == nil
			})
			if err != nil {
				return err
			}
		case f.fd.IsList():
			list := f.v.List()
			for i := range list.Len() {
				if err := visitChasmValue(ctx, opts, fmt.Sprintf("%s[%d]", fieldPath, i), f.fd, f.fd, list.Get(i)); err != nil {
					return err
				}
			}
		default:
			if err := visitChasmValue(ctx, opts, fieldPath, f.fd, f.fd, f.v); err != nil {
				return err
			}
		}
	}

	return nil
}

// visitChasmValue visits one value of field fd, whose kind kd gives: the field
// itself, or its map value. Only messages and bytes can hold anything.
func visitChasmValue(ctx context.Context, opts AdminVisitOptions, path string, fd, kd protoreflect.FieldDescriptor, v protoreflect.Value) error {
	switch kd.Kind() {
	case protoreflect.MessageKind, protoreflect.GroupKind:
		return visitChasmMessage(ctx, opts, path, fd, v.Message())
	case protoreflect.BytesKind:
		if chasmIgnoredFields[fd.FullName()] {
			return nil
		}
		return visitOpaque(opts, path, v.Bytes())
	default:
		return nil
	}
}

// visitChasmPublic visits m, a public API message inside CHASM data, as the
// proxy's own type for it, and copies the result back. A bare Payload goes to
// the payload visitor directly, skipped in a search_attributes map when search
// attributes are; anything else goes to proxy.VisitPayloads and
// proxy.VisitFailures. A type the proxy does not link, or fields its type does
// not know, cannot be seen into and are reported as opaque.
func visitChasmPublic(ctx context.Context, opts AdminVisitOptions, path string, fd protoreflect.FieldDescriptor, m protoreflect.Message) error {
	name := m.Descriptor().FullName()
	if name == payloadFullName && fd != nil && fd.IsMap() && fd.Name() == "search_attributes" && opts.PayloadOptions.SkipSearchAttributes {
		return nil
	}

	in, err := proto.Marshal(m.Interface())
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	mt, err := protoregistry.GlobalTypes.FindMessageByName(name)
	if err != nil {
		return visitOpaque(opts, path, in)
	}
	typed := mt.New().Interface()
	if err := proto.Unmarshal(in, typed); err != nil {
		return visitOpaque(opts, path, in)
	}
	if err := visitUnknownFields(opts, path, typed.ProtoReflect()); err != nil {
		return err
	}

	if p, ok := typed.(*common.Payload); ok {
		err = visitPayloadInPlace(ctx, opts, p)
	} else {
		err = visitPublic(ctx, opts, typed)
	}
	if err != nil {
		return err
	}

	out, err := proto.Marshal(typed)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	// Unmarshal resets m first, so nothing of the old value is left behind.
	if err := proto.Unmarshal(out, m.Interface()); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	return nil
}

// visitChasmAny visits the message inside m, an Any, when fd may hold one and
// its type is embedded, and packs it back. Any other Any is opaque.
func visitChasmAny(ctx context.Context, opts AdminVisitOptions, path string, fd protoreflect.FieldDescriptor, m protoreflect.Message) error {
	urlField := m.Descriptor().Fields().ByName("type_url")
	valueField := m.Descriptor().Fields().ByName("value")
	value := m.Get(valueField).Bytes()

	if fd == nil || !chasmAnyFields[fd.FullName()] {
		return visitOpaque(opts, path, value)
	}

	url := m.Get(urlField).String()
	name := protoreflect.FullName(url[strings.LastIndex(url, "/")+1:])
	files, err := chasmFiles()
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	d, err := files.FindDescriptorByName(name)
	md, ok := d.(protoreflect.MessageDescriptor)
	if err != nil || !ok {
		return visitOpaque(opts, path, value)
	}

	inner := dynamicpb.NewMessage(md)
	if err := proto.Unmarshal(value, inner); err != nil {
		return visitOpaque(opts, path, value)
	}
	if err := visitChasmMessage(ctx, opts, path+"/value", nil, inner); err != nil {
		return err
	}

	out, err := proto.MarshalOptions{Deterministic: true}.Marshal(inner)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	m.Set(valueField, protoreflect.ValueOfBytes(out))

	return nil
}

// visitUnknownFields reports the unknown fields of m and every message in it
// as opaque: they are what the proxy's types do not know, so cannot be seen into.
func visitUnknownFields(opts AdminVisitOptions, path string, m protoreflect.Message) error {
	if err := visitOpaque(opts, path+"/<unknown fields>", m.GetUnknown()); err != nil {
		return err
	}

	var err error
	m.Range(func(fd protoreflect.FieldDescriptor, v protoreflect.Value) bool {
		visit := func(v protoreflect.Value) bool {
			err = visitUnknownFields(opts, path+"/"+string(fd.Name()), v.Message())
			return err == nil
		}
		switch {
		case fd.IsMap():
			if fd.MapValue().Kind() == protoreflect.MessageKind {
				v.Map().Range(func(_ protoreflect.MapKey, mv protoreflect.Value) bool { return visit(mv) })
			}
		case fd.IsList():
			if fd.Kind() == protoreflect.MessageKind {
				for i := range v.List().Len() {
					if !visit(v.List().Get(i)) {
						break
					}
				}
			}
		case fd.Kind() == protoreflect.MessageKind:
			visit(v)
		}
		return err == nil
	})

	return err
}
