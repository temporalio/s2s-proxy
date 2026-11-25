package main

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"go.temporal.io/server/chasm"
	activitylib "go.temporal.io/server/chasm/lib/activity"
	callbacklib "go.temporal.io/server/chasm/lib/callback"
	"go.temporal.io/server/chasm/lib/nexusoperation"
	chasmscheduler "go.temporal.io/server/chasm/lib/scheduler"
	chasmworkflow "go.temporal.io/server/chasm/lib/workflow"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

const (
	chasmFieldTypePrefix = "chasm.Field["
	chasmMapTypePrefix   = "chasm.Map["
	// chasmFieldNameTag renames a CHASM child node, as chasm.fieldsOf reads it.
	chasmFieldNameTag = "name"
)

var protoMessageT = reflect.TypeFor[proto.Message]()

type (
	// chasmTables classifies the fields of server-internal CHASM messages that
	// the proxy cannot visit through their type alone: bytes, DataBlob, and Any.
	// Public API messages are visited by the payload visitor and are not listed.
	chasmTables struct {
		// AnyContents are the messages an Any in CHASM state is known to hold.
		// Their files are embedded and they are walked like the rest.
		AnyContents []protoreflect.FullName
		// AnyFields are decoded when their type resolves to an embedded message,
		// and reported as opaque otherwise.
		AnyFields map[protoreflect.FullName]bool
		// Ignored fields hold nothing that needs sealing; the value says why.
		Ignored map[protoreflect.FullName]string
		// Opaque fields are reported as opaque data.
		Opaque map[protoreflect.FullName]bool
	}

	chasmComponent struct {
		ID       uint32
		FQN      string
		State    protoreflect.FullName
		Children []chasmChild
	}

	chasmChild struct {
		Name       string
		Collection bool
		// Component is the child's component type ID, when it is a component.
		Component uint32
		// Data is the child's message, when it is a data node.
		Data protoreflect.FullName
	}

	chasmTask struct {
		ID   uint32
		FQN  string
		Data protoreflect.FullName
	}

	chasmSchema struct {
		components []chasmComponent
		tasks      []chasmTask
		// roots are every message the runtime decodes a blob into.
		roots []protoreflect.MessageDescriptor
	}
)

var adminChasmTables = chasmTables{
	AnyContents: []protoreflect.FullName{
		// The CHASM workflow packs these into a Nexus operation it starts or cancels.
		"temporal.server.chasm.lib.workflow.proto.v1.NexusOperationParentData",
		"temporal.server.chasm.lib.workflow.proto.v1.NexusCancellationParentData",
	},
	AnyFields: map[protoreflect.FullName]bool{
		"temporal.server.chasm.lib.nexusoperation.proto.v1.OperationState.parent_data":    true,
		"temporal.server.chasm.lib.nexusoperation.proto.v1.CancellationState.parent_data": true,
	},
	Ignored: map[protoreflect.FullName]string{
		"temporal.server.chasm.lib.workflow.proto.v1.NexusOperationParentData.scheduled_event_token": "serialized HistoryEventRef (event and batch IDs)",
	},
	Opaque: map[protoreflect.FullName]bool{},
}

// chasmLibraries returns the CHASM libraries the server registers, built
// without task handlers, as tdbg builds them. The proxy only reads their types.
func chasmLibraries() []chasm.Library {
	return []chasm.Library{
		&chasm.CoreLibrary{},
		chasmworkflow.NewLibrary(chasmworkflow.NewRegistry()),
		activitylib.NewNilLibrary(),
		chasmscheduler.NewNilLibrary(),
		callbacklib.NewNilLibrary(),
		// No nil constructor exists; the zero value registers the same types.
		&nexusoperation.Library{},
	}
}

// buildChasmSchema returns the source of the CHASM schema the admin payload
// visitor uses to decode CHASM state, or every problem found on the way.
func buildChasmSchema(logger log.Logger, tables chasmTables) ([]byte, error) {
	schema, err := readChasmSchema(logger, chasmLibraries())
	if err != nil {
		return nil, err
	}

	anyRoots := make([]protoreflect.MessageDescriptor, 0, len(tables.AnyContents))
	var errs []error
	for _, name := range tables.AnyContents {
		md, err := findMessage(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("any content %s: %w", name, err))
			continue
		}
		anyRoots = append(anyRoots, md)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	roots := slices.Concat(schema.roots, anyRoots)
	if err := classifyChasmFields(roots, tables); err != nil {
		return nil, err
	}

	embedded, imports, err := chasmFiles(roots)
	if err != nil {
		return nil, err
	}

	return emitChasmSchema(schema, tables, embedded, imports)
}

// readChasmSchema registers libs and reads every component's state message and
// children, and every task's message, the way the server's tree reads them.
func readChasmSchema(logger log.Logger, libs []chasm.Library) (*chasmSchema, error) {
	registry := chasm.NewRegistry(logger)
	for _, lib := range libs {
		if err := registry.Register(lib); err != nil {
			return nil, fmt.Errorf("register CHASM library %s: %w", lib.Name(), err)
		}
	}

	schema := &chasmSchema{}
	seenRoot := map[protoreflect.FullName]bool{}
	addRoot := func(name protoreflect.FullName) {
		if name == "" || seenRoot[name] {
			return
		}
		seenRoot[name] = true
		md, _ := findMessage(name)
		schema.roots = append(schema.roots, md)
	}

	var errs []error
	for _, lib := range libs {
		for _, rc := range lib.Components() {
			c, err := readChasmComponent(registry, rc.GoType())
			if err != nil {
				errs = append(errs, err)
				continue
			}
			schema.components = append(schema.components, c)
			addRoot(c.State)
			for _, ch := range c.Children {
				addRoot(ch.Data)
			}
		}

		for _, rt := range lib.Tasks() {
			t, err := readChasmTask(registry, rt.GoType())
			if err != nil {
				errs = append(errs, err)
				continue
			}
			schema.tasks = append(schema.tasks, t)
			addRoot(t.Data)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	slices.SortFunc(schema.components, func(a, b chasmComponent) int { return strings.Compare(a.FQN, b.FQN) })
	slices.SortFunc(schema.tasks, func(a, b chasmTask) int { return strings.Compare(a.FQN, b.FQN) })
	slices.SortFunc(schema.roots, func(a, b protoreflect.MessageDescriptor) int {
		return strings.Compare(string(a.FullName()), string(b.FullName()))
	})

	return schema, nil
}

func readChasmComponent(registry *chasm.Registry, goType reflect.Type) (chasmComponent, error) {
	if goType.Kind() != reflect.Pointer || goType.Elem().Kind() != reflect.Struct {
		return chasmComponent{}, fmt.Errorf("CHASM component %s is not a pointer to a struct", goType)
	}

	id, ok := registry.ComponentIDFor(reflect.New(goType.Elem()).Interface())
	if !ok {
		return chasmComponent{}, fmt.Errorf("CHASM component %s has no type ID", goType)
	}
	fqn, _ := registry.ComponentFqnByID(id)
	c := chasmComponent{ID: id, FQN: fqn}

	st := goType.Elem()
	for i := range st.NumField() {
		f := st.Field(i)
		ft := f.Type
		name := f.Name
		if tag := f.Tag.Get(chasmFieldNameTag); tag != "" {
			name = tag
		}

		// The one field holding a proto message is the component's state.
		if ft.AssignableTo(protoMessageT) {
			if c.State != "" {
				return chasmComponent{}, fmt.Errorf("CHASM component %s has more than one state field", fqn)
			}
			c.State = messageName(ft)
			continue
		}

		var collection bool
		switch {
		case strings.HasPrefix(ft.String(), chasmFieldTypePrefix):
		case strings.HasPrefix(ft.String(), chasmMapTypePrefix):
			collection = true
			ft = ft.Elem()
		default:
			continue
		}

		get, ok := ft.MethodByName("Get")
		if !ok || get.Type.NumOut() == 0 {
			return chasmComponent{}, fmt.Errorf("CHASM component %s field %s: %s has no Get method", fqn, name, ft)
		}
		child := chasmChild{Name: name, Collection: collection}
		t := get.Type.Out(0)
		if cid, ok := componentIDOf(registry, t); ok {
			child.Component = cid
		} else if t.Implements(protoMessageT) {
			child.Data = messageName(t)
		} else {
			return chasmComponent{}, fmt.Errorf("CHASM component %s field %s holds %s, which is neither a component nor a proto message", fqn, name, t)
		}
		c.Children = append(c.Children, child)
	}

	slices.SortFunc(c.Children, func(a, b chasmChild) int { return strings.Compare(a.Name, b.Name) })
	return c, nil
}

// readChasmTask reads the message a task's data decodes to, the way the
// server's deserializeTask finds it: the task itself, or its one proto field.
// A task with neither has no data.
func readChasmTask(registry *chasm.Registry, goType reflect.Type) (chasmTask, error) {
	instance := reflect.Zero(goType).Interface()
	if goType.Kind() == reflect.Pointer {
		instance = reflect.New(goType.Elem()).Interface()
	}
	id, ok := registry.TaskIDFor(instance)
	if !ok {
		return chasmTask{}, fmt.Errorf("CHASM task %s has no type ID", goType)
	}
	fqn, _ := registry.TaskFqnByID(id)
	t := chasmTask{ID: id, FQN: fqn}

	if goType.Implements(protoMessageT) {
		t.Data = messageName(goType)
		return t, nil
	}

	st := goType
	if st.Kind() == reflect.Pointer {
		st = st.Elem()
	}
	if st.Kind() != reflect.Struct {
		return chasmTask{}, fmt.Errorf("CHASM task %s is neither a proto message nor a struct", fqn)
	}
	for i := range st.NumField() {
		ft := st.Field(i).Type
		if !ft.AssignableTo(protoMessageT) {
			continue
		}
		if t.Data != "" {
			return chasmTask{}, fmt.Errorf("CHASM task %s has more than one proto field", fqn)
		}
		t.Data = messageName(ft)
	}

	return t, nil
}

func componentIDOf(registry *chasm.Registry, t reflect.Type) (uint32, bool) {
	if t.Kind() != reflect.Pointer {
		return 0, false
	}
	return registry.ComponentIDFor(reflect.New(t.Elem()).Interface())
}

// messageName returns the full name of the proto message type t points to.
func messageName(t reflect.Type) protoreflect.FullName {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return reflect.New(t).Interface().(proto.Message).ProtoReflect().Descriptor().FullName()
}

func findMessage(name protoreflect.FullName) (protoreflect.MessageDescriptor, error) {
	mt, err := protoregistry.GlobalTypes.FindMessageByName(name)
	if err != nil {
		return nil, err
	}
	return mt.Descriptor(), nil
}

func isPublicOrWellKnown(name protoreflect.FullName) bool {
	return strings.HasPrefix(string(name), publicAPIPrefix) || strings.HasPrefix(string(name), "google.protobuf.")
}

// classifyChasmFields requires every bytes, DataBlob, and Any field in the
// server-internal messages below roots to be in exactly one of tables'
// classifications, and every classification to name such a field.
func classifyChasmFields(roots []protoreflect.MessageDescriptor, tables chasmTables) error {
	found := map[protoreflect.FullName]bool{}
	seen := map[protoreflect.FullName]bool{}

	var walk func(md protoreflect.MessageDescriptor)
	walk = func(md protoreflect.MessageDescriptor) {
		if seen[md.FullName()] || isPublicOrWellKnown(md.FullName()) {
			return
		}
		seen[md.FullName()] = true

		fields := md.Fields()
		for i := range fields.Len() {
			f := fields.Get(i)
			vf := f
			if f.IsMap() {
				vf = f.MapValue()
			}
			switch {
			case vf.Kind() == protoreflect.BytesKind:
				found[f.FullName()] = true
			case vf.Kind() == protoreflect.MessageKind:
				switch vf.Message().FullName() {
				case dataBlobMessage, anyMessage:
					found[f.FullName()] = true
				default:
					walk(vf.Message())
				}
			}
		}
	}
	for _, md := range roots {
		walk(md)
	}

	var errs []error
	for _, name := range slices.Sorted(maps.Keys(found)) {
		n := 0
		if tables.AnyFields[name] {
			n++
		}
		if _, ok := tables.Ignored[name]; ok {
			n++
		}
		if tables.Opaque[name] {
			n++
		}
		switch n {
		case 0:
			errs = append(errs, fmt.Errorf("unclassified CHASM field %s; add it to AnyFields, Ignored, or Opaque", name))
		case 1:
		default:
			errs = append(errs, fmt.Errorf("CHASM field %s is in more than one table", name))
		}
	}

	for _, names := range [][]protoreflect.FullName{keys(tables.AnyFields), keys(tables.Ignored), keys(tables.Opaque)} {
		for _, name := range names {
			if !found[name] {
				errs = append(errs, fmt.Errorf("CHASM field %s never matched; fix or remove it", name))
			}
		}
	}
	slices.SortFunc(errs, func(a, b error) int { return strings.Compare(a.Error(), b.Error()) })

	return errors.Join(errs...)
}

// chasmFiles returns the files the roots live in and everything they import,
// split into those to embed, in dependency order, and the Go packages of the
// public and well-known ones, which the proxy links and resolves from the
// global registry instead.
func chasmFiles(roots []protoreflect.MessageDescriptor) ([]*descriptorpb.FileDescriptorProto, []string, error) {
	var (
		embedded []*descriptorpb.FileDescriptorProto
		imports  = map[string]bool{}
		seen     = map[string]bool{}
		errs     []error
	)

	var visit func(fd protoreflect.FileDescriptor)
	visit = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true

		fdp := protodesc.ToFileDescriptorProto(fd)
		if strings.HasPrefix(fd.Path(), "temporal/api/") || strings.HasPrefix(fd.Path(), "google/protobuf/") {
			pkg, _, _ := strings.Cut(fdp.GetOptions().GetGoPackage(), ";")
			if pkg == "" {
				errs = append(errs, fmt.Errorf("%s has no go_package", fd.Path()))
				return
			}
			imports[pkg] = true
			return
		}

		// Imports first, so each file can be built once everything it needs is.
		deps := fd.Imports()
		for i := range deps.Len() {
			visit(deps.Get(i).FileDescriptor)
		}
		fdp.SourceCodeInfo = nil
		embedded = append(embedded, fdp)
	}

	files := map[string]protoreflect.FileDescriptor{}
	for _, md := range roots {
		files[md.ParentFile().Path()] = md.ParentFile()
	}
	for _, path := range slices.Sorted(maps.Keys(files)) {
		visit(files[path])
	}

	return embedded, slices.Sorted(maps.Keys(imports)), errors.Join(errs...)
}

func emitChasmSchema(schema *chasmSchema, tables chasmTables, embedded []*descriptorpb.FileDescriptorProto, imports []string) ([]byte, error) {
	set, err := proto.MarshalOptions{Deterministic: true}.Marshal(&descriptorpb.FileDescriptorSet{File: embedded})
	if err != nil {
		return nil, fmt.Errorf("marshal CHASM descriptors: %w", err)
	}

	var out bytes.Buffer
	writeln(&out, `// Code generated by cmd/tools/genvisitor. DO NOT EDIT.`)
	writeln(&out, "package interceptor")
	writeln(&out)
	writeln(&out, "import (")
	writeln(&out, `"google.golang.org/protobuf/reflect/protoreflect"`)
	writeln(&out)
	writeln(&out, "// The public and well-known files the CHASM descriptors import, which")
	writeln(&out, "// are resolved from the global registry and so must be linked.")
	for _, imp := range imports {
		writef(&out, "_ %q\n", imp)
	}
	writeln(&out, ")")
	writeln(&out)

	writeln(&out, "// chasmComponents are the CHASM components the server registers, by type ID.")
	writeln(&out, "var chasmComponents = map[uint32]chasmComponentSchema{")
	for _, c := range schema.components {
		writef(&out, "%d: {\nfqn: %q,\n", c.ID, c.FQN)
		if c.State != "" {
			writef(&out, "state: %q,\n", c.State)
		}
		if len(c.Children) > 0 {
			writeln(&out, "children: map[string]chasmChildSchema{")
			for _, ch := range c.Children {
				writef(&out, "%q: {", ch.Name)
				var parts []string
				if ch.Collection {
					parts = append(parts, "collection: true")
				}
				if ch.Data != "" {
					parts = append(parts, fmt.Sprintf("data: %q", ch.Data))
				} else {
					parts = append(parts, fmt.Sprintf("component: %d", ch.Component))
				}
				writef(&out, "%s},\n", strings.Join(parts, ", "))
			}
			writeln(&out, "},")
		}
		writeln(&out, "},")
	}
	writeln(&out, "}")
	writeln(&out)

	writeln(&out, "// chasmTasks are the CHASM tasks the server registers, by type ID.")
	writeln(&out, "var chasmTasks = map[uint32]chasmTaskSchema{")
	for _, t := range schema.tasks {
		writef(&out, "%d: {fqn: %q", t.ID, t.FQN)
		if t.Data != "" {
			writef(&out, ", data: %q", t.Data)
		}
		writeln(&out, "},")
	}
	writeln(&out, "}")
	writeln(&out)

	emitNameSet := func(doc, name string, names []protoreflect.FullName) {
		writef(&out, "// %s\n", doc)
		writef(&out, "var %s = map[protoreflect.FullName]bool{\n", name)
		slices.Sort(names)
		for _, n := range names {
			writef(&out, "%q: true,\n", n)
		}
		writeln(&out, "}")
		writeln(&out)
	}
	emitNameSet("chasmAnyFields are Any fields decoded when their type is embedded.", "chasmAnyFields", keys(tables.AnyFields))
	emitNameSet("chasmIgnoredFields hold nothing that needs sealing.", "chasmIgnoredFields", keys(tables.Ignored))

	writeln(&out, "// chasmDescriptorSet is a serialized FileDescriptorSet of the server-internal")
	writeln(&out, "// files CHASM state is declared in, in dependency order.")
	writeln(&out, "const chasmDescriptorSet = \"\" +")
	const chunk = 48
	for i := 0; i < len(set); i += chunk {
		end := min(i+chunk, len(set))
		sep := " +"
		if end == len(set) {
			sep = ""
		}
		writef(&out, "%q%s\n", set[i:end], sep)
	}

	return out.Bytes(), nil
}
