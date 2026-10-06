package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const (
	adminServicePackage protoreflect.FullName = "temporal.server.api.adminservice.v1"
	publicAPIPrefix                           = "temporal.api."
	payloadMessage      protoreflect.FullName = "temporal.api.common.v1.Payload"
	dataBlobMessage     protoreflect.FullName = "temporal.api.common.v1.DataBlob"
	anyMessage          protoreflect.FullName = "google.protobuf.Any"
)

type (
	// payloadTables classifies everything the payload walk cannot see through by
	// itself. Every bytes, DataBlob, or Any field reachable from an admin message
	// must appear in exactly one table, and every entry must match something.
	payloadTables struct {
		// EventBlobs are DataBlob fields holding serialized history events.
		EventBlobs map[protoreflect.FullName]bool
		// Opaque are fields whose contents the proxy cannot decode. Non-empty ones
		// are reported to AdminVisitOptions.Opaque.
		Opaque map[protoreflect.FullName]bool
		// Helpers maps a message the generated code cannot walk, because it recurses
		// or its fields only make sense together, to the hand-written helper that
		// walks it.
		Helpers map[protoreflect.FullName]payloadHelper
		// Ignored maps a field that cannot hold payloads to the reason why.
		Ignored map[protoreflect.FullName]string
		// DecodeTargets are messages a helper decodes out of a blob and hands back
		// to visitAdminPayloads, so they are walked as roots alongside the admin
		// messages.
		DecodeTargets []protoreflect.FullName
	}

	// payloadHelper names the hand-written function that walks a message, and
	// every field it reads. Any other field of that message, or of a message
	// reached through a handled field, must be inert, because the helper never
	// looks at it.
	payloadHelper struct {
		Name    string
		Handles []protoreflect.FullName
	}

	// payloadClassifier decides what the payloads target does at each node of the
	// walk and remembers which table entries it used.
	payloadClassifier struct {
		tables payloadTables
		used   map[protoreflect.FullName]bool
		reach  map[protoreflect.FullName]bool
		// inertMemo caches isInert answers that did not depend on a cycle.
		inertMemo map[protoreflect.FullName]bool
	}
)

var adminPayloadTables = payloadTables{
	EventBlobs: map[protoreflect.FullName]bool{
		"temporal.server.api.replication.v1.HistoryTaskAttributes.events":                              true,
		"temporal.server.api.replication.v1.HistoryTaskAttributes.new_run_events":                      true,
		"temporal.server.api.replication.v1.HistoryTaskAttributes.events_batches":                      true,
		"temporal.server.api.replication.v1.BackfillHistoryTaskAttributes.event_batches":               true,
		"temporal.server.api.replication.v1.NewRunInfo.event_batch":                                    true,
		"temporal.server.api.replication.v1.VersionedTransitionArtifact.event_batches":                 true,
		"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryV2Response.history_batches": true,
		"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryResponse.history_batches":   true,
		"temporal.server.api.adminservice.v1.ReapplyEventsRequest.events":                              true,
		"temporal.server.api.adminservice.v1.ImportWorkflowExecutionRequest.history_batches":           true,
	},
	Opaque: map[protoreflect.FullName]bool{
		// CHASM component state. The server decodes it, so it cannot be sealed
		// whole, and decoding it here needs the server's component registry.
		"temporal.server.api.persistence.v1.ChasmNode.data": true,
		// Documented as the future home of every task attribute.
		"temporal.server.api.replication.v1.ReplicationTask.data": true,
		// Serialized mutable state, handed back and forth between import calls.
		"temporal.server.api.adminservice.v1.ImportWorkflowExecutionRequest.token":  true,
		"temporal.server.api.adminservice.v1.ImportWorkflowExecutionResponse.token": true,
		// HSM node data in a mutation, the same thing StateMachineNode.data holds.
		"temporal.server.api.persistence.v1.WorkflowMutableStateMutation.StateMachineNodeMutation.data": true,
		// HSM and CHASM task data. CHASM types come from the server's component
		// registry, which the proxy does not have.
		"temporal.server.api.persistence.v1.StateMachineTaskInfo.data":          true,
		"temporal.server.api.persistence.v1.ChasmComponentAttributes.Task.data": true,
		"temporal.server.api.persistence.v1.ChasmTaskInfo.data":                 true,
		// A serialized history task. Its category, which says what it decodes to,
		// is on the GetDLQTasksRequest, and the response is visited on its own.
		"temporal.server.api.common.v1.HistoryTask.blob": true,
	},
	Helpers: map[protoreflect.FullName]payloadHelper{
		// A serialized history task, which decodes to one of DecodeTargets
		// depending on its category.
		"temporal.server.api.adminservice.v1.AddTasksRequest.Task": {
			Name: "visitAddTasksRequestTask",
			Handles: []protoreflect.FullName{
				"temporal.server.api.adminservice.v1.AddTasksRequest.Task.blob",
			},
		},
		// HSM nodes nest through children; every node's data is opaque.
		"temporal.server.api.persistence.v1.StateMachineNode": {
			Name: "visitStateMachineNode",
			Handles: []protoreflect.FullName{
				"temporal.server.api.persistence.v1.StateMachineNode.data",
				"temporal.server.api.persistence.v1.StateMachineNode.children",
				"temporal.server.api.persistence.v1.StateMachineMap.machines_by_id",
			},
		},
	},
	Ignored: map[protoreflect.FullName]string{
		"temporal.server.api.adminservice.v1.CancelDLQJobRequest.job_token":                            "DLQ job cursor",
		"temporal.server.api.adminservice.v1.DescribeDLQJobRequest.job_token":                          "DLQ job cursor",
		"temporal.server.api.adminservice.v1.MergeDLQTasksResponse.job_token":                          "DLQ job cursor",
		"temporal.server.api.adminservice.v1.PurgeDLQTasksResponse.job_token":                          "DLQ job cursor",
		"temporal.server.api.adminservice.v1.GetDLQMessagesRequest.next_page_token":                    "pagination cursor",
		"temporal.server.api.adminservice.v1.GetDLQMessagesResponse.next_page_token":                   "pagination cursor",
		"temporal.server.api.adminservice.v1.GetDLQTasksRequest.next_page_token":                       "pagination cursor",
		"temporal.server.api.adminservice.v1.GetDLQTasksResponse.next_page_token":                      "pagination cursor",
		"temporal.server.api.adminservice.v1.GetTaskQueueTasksRequest.next_page_token":                 "pagination cursor",
		"temporal.server.api.adminservice.v1.GetTaskQueueTasksResponse.next_page_token":                "pagination cursor",
		"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryRequest.next_page_token":    "pagination cursor",
		"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryResponse.next_page_token":   "pagination cursor",
		"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryV2Request.next_page_token":  "pagination cursor",
		"temporal.server.api.adminservice.v1.GetWorkflowExecutionRawHistoryV2Response.next_page_token": "pagination cursor",
		"temporal.server.api.adminservice.v1.ListClusterMembersRequest.next_page_token":                "pagination cursor",
		"temporal.server.api.adminservice.v1.ListClusterMembersResponse.next_page_token":               "pagination cursor",
		"temporal.server.api.adminservice.v1.ListClustersRequest.next_page_token":                      "pagination cursor",
		"temporal.server.api.adminservice.v1.ListClustersResponse.next_page_token":                     "pagination cursor",
		"temporal.server.api.adminservice.v1.ListHistoryTasksRequest.next_page_token":                  "pagination cursor",
		"temporal.server.api.adminservice.v1.ListHistoryTasksResponse.next_page_token":                 "pagination cursor",
		"temporal.server.api.adminservice.v1.ListQueuesRequest.next_page_token":                        "pagination cursor",
		"temporal.server.api.adminservice.v1.ListQueuesResponse.next_page_token":                       "pagination cursor",
		"temporal.server.api.adminservice.v1.MergeDLQMessagesRequest.next_page_token":                  "pagination cursor",
		"temporal.server.api.adminservice.v1.MergeDLQMessagesResponse.next_page_token":                 "pagination cursor",
		"temporal.server.api.history.v1.VersionHistory.branch_token":                                   "serialized history branch IDs",
		"temporal.server.api.persistence.v1.ReplicationTaskInfo.branch_token":                          "serialized history branch IDs",
		"temporal.server.api.persistence.v1.ReplicationTaskInfo.new_run_branch_token":                  "serialized history branch IDs",
		"temporal.server.api.persistence.v1.TimerTaskInfo.branch_token":                                "serialized history branch IDs",
		"temporal.server.api.persistence.v1.Checksum.value":                                            "mutable state checksum",
		"temporal.server.api.persistence.v1.TaskInfo.component_ref":                                    "serialized CHASM component reference (IDs and a path)",
		"temporal.server.api.taskqueue.v1.PartitionScaleInfo.backlog_counts":                           "packed per-partition counters",
	},
	// What a serialized history task decodes to, one per persisted category.
	DecodeTargets: []protoreflect.FullName{
		"temporal.server.api.persistence.v1.ArchivalTaskInfo",
		"temporal.server.api.persistence.v1.OutboundTaskInfo",
		"temporal.server.api.persistence.v1.ReplicationTaskInfo",
		"temporal.server.api.persistence.v1.TimerTaskInfo",
		"temporal.server.api.persistence.v1.TransferTaskInfo",
		"temporal.server.api.persistence.v1.VisibilityTaskInfo",
	},
}

// buildPayloads returns an emitter for visitAdminPayloads, or every
// classification problem found on the way.
func buildPayloads(logger log.Logger, tables payloadTables) (*Emitter, error) {
	c, err := newPayloadClassifier(tables)
	if err != nil {
		return nil, err
	}

	e := NewEmitter(logger, CurrentVersion)
	e.SetPackageName("interceptor")
	e.SetFunctionSignature("func visitAdminPayloads(ctx context.Context, vAny any, opts AdminVisitOptions) error")
	e.SetFunctionTrailer("return nil")
	e.AddImport("context")

	e.AddHandler(c.isPublicMessage, true, func(v string, _ VisitPath) string {
		return returnOnErr(fmt.Sprintf("visitMessage(ctx, opts, %s)", v))
	})
	e.AddHandler(c.isServerPayload, true, func(v string, path VisitPath) string {
		call := returnOnErr(fmt.Sprintf("visitPayloadInPlace(ctx, opts, %s)", v))
		if isSearchAttributes(path) {
			return fmt.Sprintf("if !opts.Payloads.SkipSearchAttributes {\n%s\n}", call)
		}
		return call
	})
	e.AddHandler(c.isEventBlob, true, func(v string, _ VisitPath) string {
		return returnOnErr(fmt.Sprintf("visitEventBlob(ctx, opts, %s)", v))
	})
	e.AddHandler(c.isOpaque, true, func(v string, path VisitPath) string {
		data := v + ".GetData()"
		if f, _ := path[len(path)-1].AsField(); f.Kind() == protoreflect.BytesKind {
			data = v
		}
		return returnOnErr(fmt.Sprintf("visitOpaque(opts, %q, %s)", path.String(), data))
	})
	e.AddHandler(c.isHelper, true, func(v string, path VisitPath) string {
		md, _ := path[len(path)-1].AsMessage()
		return returnOnErr(fmt.Sprintf("%s(ctx, opts, %q, %s)", tables.Helpers[md.FullName()].Name, path.String(), v))
	})
	e.SetCheck(c.check)

	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		if mt.Descriptor().ParentFile().Package() == adminServicePackage {
			e.Visit(mt)
		}
		return true
	})

	var errs []error
	for _, name := range tables.DecodeTargets {
		mt, err := protoregistry.GlobalTypes.FindMessageByName(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("decode target %s: %w", name, err))
			continue
		}
		e.Visit(mt)
	}

	if err := errors.Join(append(errs, e.Err(), c.unused(), c.checkHelpers())...); err != nil {
		return nil, err
	}

	return e, nil
}

func newPayloadClassifier(tables payloadTables) (*payloadClassifier, error) {
	seen := map[protoreflect.FullName]bool{}
	var errs []error
	for _, names := range [][]protoreflect.FullName{
		keys(tables.EventBlobs), keys(tables.Opaque), keys(tables.Helpers), keys(tables.Ignored),
	} {
		for _, n := range names {
			if seen[n] {
				errs = append(errs, fmt.Errorf("%s is in more than one table", n))
			}
			seen[n] = true
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	return &payloadClassifier{
		tables:    tables,
		used:      map[protoreflect.FullName]bool{},
		reach:     map[protoreflect.FullName]bool{},
		inertMemo: map[protoreflect.FullName]bool{},
	}, nil
}

// isPublicMessage matches the first public API message on a path that can hold
// a payload. proxy.VisitPayloads covers everything below it. Payload itself is
// left to isServerPayload, because proxy.VisitPayloads does not accept one.
func (c *payloadClassifier) isPublicMessage(vt VisitType, _ VisitPath) bool {
	md, ok := vt.AsMessage()
	return ok && !vt.Cycle &&
		strings.HasPrefix(string(md.FullName()), publicAPIPrefix) &&
		md.FullName() != payloadMessage &&
		c.canReachPayload(md)
}

// isServerPayload matches a Payload held directly by a server-internal message.
// Public parents never get here, isPublicMessage having matched them first.
func (c *payloadClassifier) isServerPayload(vt VisitType, _ VisitPath) bool {
	md, ok := vt.AsMessage()
	return ok && md.FullName() == payloadMessage
}

func (c *payloadClassifier) isEventBlob(vt VisitType, _ VisitPath) bool {
	return c.matchField(vt, c.tables.EventBlobs)
}

func (c *payloadClassifier) isOpaque(vt VisitType, _ VisitPath) bool {
	return c.matchField(vt, c.tables.Opaque)
}

func (c *payloadClassifier) isHelper(vt VisitType, _ VisitPath) bool {
	md, ok := vt.AsMessage()
	if !ok || vt.Cycle {
		return false
	}
	if _, ok := c.tables.Helpers[md.FullName()]; ok {
		c.used[md.FullName()] = true
		return true
	}
	return false
}

func (c *payloadClassifier) matchField(vt VisitType, table map[protoreflect.FullName]bool) bool {
	f, ok := vt.AsField()
	if ok && table[f.FullName()] {
		c.used[f.FullName()] = true
		return true
	}
	return false
}

// check rejects what no handler matched but could still hide a payload:
// recursion the generated code cannot unroll, and bytes, DataBlob, or Any fields
// with no classification. Public messages that cannot hold a payload are pruned,
// since proxy.VisitPayloads owns everything public.
func (c *payloadClassifier) check(vt VisitType, path VisitPath, matched bool) (Action, error) {
	if vt.Cycle {
		// The first occurrence was walked in full, so deeper ones only matter if
		// something below them would generate code.
		if md, ok := vt.AsMessage(); ok && c.isInert(md) {
			return Prune, nil
		}
		return Prune, fmt.Errorf("unhandled recursion: %s at %s; add it to Helpers with a hand-written helper", vt.FullName(), path)
	}
	if matched {
		return Descend, nil
	}

	if md, ok := vt.AsMessage(); ok {
		if strings.HasPrefix(string(md.FullName()), publicAPIPrefix) {
			return Prune, nil
		}
		return Descend, nil
	}

	f, ok := vt.AsField()
	if !ok {
		return Descend, nil
	}
	if _, ok := c.tables.Ignored[f.FullName()]; ok {
		c.used[f.FullName()] = true
		return Prune, nil
	}
	if isOpaqueKind(f) {
		return Prune, fmt.Errorf("unclassified field %s at %s; add it to EventBlobs, Opaque, or Ignored", f.FullName(), path)
	}

	return Descend, nil
}

// checkHelpers requires every field a helper does not read to be inert, so that
// a field added to the type later cannot slip past the helper.
// Handled fields that hold messages pull those messages into the check too,
// except bytes, DataBlob, and Any fields, which the helper takes whole.
func (c *payloadClassifier) checkHelpers() error {
	var errs []error
	for name, helper := range c.tables.Helpers {
		handled := map[protoreflect.FullName]bool{}
		for _, f := range helper.Handles {
			handled[f] = false
		}

		d, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
		if err != nil {
			errs = append(errs, fmt.Errorf("helper type %s: %w", name, err))
			continue
		}
		md, ok := d.(protoreflect.MessageDescriptor)
		if !ok {
			errs = append(errs, fmt.Errorf("helper type %s is not a message", name))
			continue
		}

		queue := []protoreflect.MessageDescriptor{md}
		seen := map[protoreflect.FullName]bool{name: true}
		for len(queue) > 0 {
			t := queue[0]
			queue = queue[1:]

			fields := t.Fields()
			for i := range fields.Len() {
				f := fields.Get(i)
				if _, ok := handled[f.FullName()]; ok {
					handled[f.FullName()] = true
					if isOpaqueKind(f) {
						continue
					}
					vf := f
					if f.IsMap() {
						vf = f.MapValue()
					}
					if vf.Kind() == protoreflect.MessageKind && !seen[vf.Message().FullName()] {
						seen[vf.Message().FullName()] = true
						queue = append(queue, vf.Message())
					}
					continue
				}
				if !c.isInertField(f) {
					errs = append(errs, fmt.Errorf("%s is not handled by %s and could hold data; handle it there and list it in Handles", f.FullName(), helper.Name))
				}
			}
		}

		for f, found := range handled {
			if !found {
				errs = append(errs, fmt.Errorf("%s never matched; fix or remove it from the Handles of %s", f, name))
			}
		}
	}

	return errors.Join(errs...)
}

// isInertField reports whether f can hold nothing that would need visiting.
func (c *payloadClassifier) isInertField(f protoreflect.FieldDescriptor) bool {
	if isOpaqueKind(f) {
		_, ignored := c.tables.Ignored[f.FullName()]
		return ignored
	}

	vf := f
	if f.IsMap() {
		vf = f.MapValue()
	}
	if vf.Kind() != protoreflect.MessageKind {
		return true
	}
	return c.isInert(vf.Message())
}

// unused reports table entries that never matched, which are typos or fields
// the server has since removed.
func (c *payloadClassifier) unused() error {
	var names []protoreflect.FullName
	for _, n := range slices.Concat(keys(c.tables.EventBlobs), keys(c.tables.Opaque), keys(c.tables.Helpers), keys(c.tables.Ignored)) {
		if !c.used[n] {
			names = append(names, n)
		}
	}
	slices.Sort(names)

	errs := make([]error, 0, len(names))
	for _, n := range names {
		errs = append(errs, fmt.Errorf("%s never matched; fix or remove it", n))
	}
	return errors.Join(errs...)
}

// canReachPayload reports whether a message of type md can hold a payload,
// directly or through any field. An Any counts, since it can hold anything.
// Only answers that did not depend on a message still being worked out are
// memoized, so a cycle cannot leave a wrong "no" behind.
func (c *payloadClassifier) canReachPayload(md protoreflect.MessageDescriptor) bool {
	ok, _ := c.reachable(md, map[protoreflect.FullName]bool{})
	return ok
}

func (c *payloadClassifier) reachable(md protoreflect.MessageDescriptor, inProgress map[protoreflect.FullName]bool) (ok, provisional bool) {
	name := md.FullName()
	if v, ok := c.reach[name]; ok {
		return v, false
	}
	if name == payloadMessage || name == anyMessage {
		c.reach[name] = true
		return true, false
	}
	if inProgress[name] {
		return false, true
	}

	inProgress[name] = true
	defer delete(inProgress, name)

	fields := md.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		if f.IsMap() {
			f = f.MapValue()
		}
		if f.Kind() != protoreflect.MessageKind {
			continue
		}
		childOK, childProvisional := c.reachable(f.Message(), inProgress)
		if childOK {
			c.reach[name] = true
			return true, false
		}
		provisional = provisional || childProvisional
	}

	if !provisional {
		c.reach[name] = false
	}
	return false, provisional
}

// isInert reports whether nothing reachable from md would generate code: no
// payload, and no bytes, DataBlob, or Any field that is not on the ignore list.
// Public messages that cannot hold a payload count as inert, matching check,
// which prunes them.
func (c *payloadClassifier) isInert(md protoreflect.MessageDescriptor) bool {
	ok, _ := c.inert(md, map[protoreflect.FullName]bool{})
	return ok
}

func (c *payloadClassifier) inert(md protoreflect.MessageDescriptor, inProgress map[protoreflect.FullName]bool) (ok, provisional bool) {
	name := md.FullName()
	if v, ok := c.inertMemo[name]; ok {
		return v, false
	}
	if c.canReachPayload(md) {
		c.inertMemo[name] = false
		return false, false
	}
	if strings.HasPrefix(string(name), publicAPIPrefix) {
		c.inertMemo[name] = true
		return true, false
	}
	if inProgress[name] {
		return true, true
	}

	inProgress[name] = true
	defer delete(inProgress, name)

	fields := md.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		if isOpaqueKind(f) {
			if _, ignored := c.tables.Ignored[f.FullName()]; !ignored {
				c.inertMemo[name] = false
				return false, false
			}
			continue
		}

		vf := f
		if f.IsMap() {
			vf = f.MapValue()
		}
		if vf.Kind() != protoreflect.MessageKind {
			continue
		}
		childOK, childProvisional := c.inert(vf.Message(), inProgress)
		if !childOK {
			c.inertMemo[name] = false
			return false, false
		}
		provisional = provisional || childProvisional
	}

	if !provisional {
		c.inertMemo[name] = true
	}
	return true, provisional
}

// isOpaqueKind reports whether f (or its map value) is a kind of field that can
// carry payloads the walk cannot see: raw bytes, a DataBlob, or an Any.
func isOpaqueKind(f protoreflect.FieldDescriptor) bool {
	if f.IsMap() {
		f = f.MapValue()
	}
	switch f.Kind() {
	case protoreflect.BytesKind:
		return true
	case protoreflect.MessageKind:
		n := f.Message().FullName()
		return n == dataBlobMessage || n == anyMessage
	default:
		return false
	}
}

// isSearchAttributes reports whether the payload at the end of path sits in a
// field named search_attributes, which stays readable so the server can index it.
func isSearchAttributes(path VisitPath) bool {
	if len(path) < 2 {
		return false
	}
	f, ok := path[len(path)-2].AsField()
	return ok && f.Name() == "search_attributes"
}

func returnOnErr(call string) string {
	return fmt.Sprintf("if err := %s; err != nil {\nreturn err\n}", call)
}

func keys[V any](m map[protoreflect.FullName]V) []protoreflect.FullName {
	out := make([]protoreflect.FullName, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
