package interceptor

import (
	"context"
	"errors"
	"fmt"

	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/proxy"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	persistence "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common/persistence/serialization"
	"go.temporal.io/server/service/history/tasks"
	"google.golang.org/protobuf/proto"

	s2scommon "github.com/temporalio/s2s-proxy/common"
)

// AdminVisitOptions configures a visit over an admin service message.
type AdminVisitOptions struct {
	// PayloadOptions is passed to proxy.VisitPayloads for every public API message
	// reached, and its Visitor is called directly for payloads held by
	// server-internal messages. SkipSearchAttributes applies to both.
	PayloadOptions *proxy.VisitPayloadsOptions
	// Opaque is called with the path and contents of every non-empty field whose
	// payloads cannot be reached, such as HSM and CHASM state. The visitor does
	// not know which direction it runs in, so the caller decides: an error stops
	// the visit, nil lets the data through as it is.
	Opaque func(path string, data []byte) error
	// FailureOptions, when set, is passed to proxy.VisitFailures for every public
	// API message reached and for the history events in every event blob, so
	// failures get the same coverage as payloads. Optional.
	FailureOptions *proxy.VisitFailuresOptions
	// FailuresAfter visits failures after payloads rather than before. Opening
	// has to open the encoded attributes before restoring them, while sealing has
	// to encode them before they are sealed.
	FailuresAfter bool
}

// VisitAdminPayloads calls opts.Payloads.Visitor for every payload in msg, an
// admin service request or response, including the replication tasks, history
// tasks, and history events inside it, and reports opaque data it cannot see into to
// opts.Opaque. Messages that are not admin service messages are left alone.
//
// Search attributes are skipped when opts.Payloads.SkipSearchAttributes is set.
func VisitAdminPayloads(ctx context.Context, msg proto.Message, opts AdminVisitOptions) error {
	if opts.PayloadOptions == nil || opts.PayloadOptions.Visitor == nil {
		return errors.New("visiting admin payloads requires a payload visitor")
	}
	if opts.Opaque == nil {
		return errors.New("visiting admin payloads requires an opaque data handler")
	}

	return visitAdminPayloads(ctx, msg, opts)
}

// namespaceIDKey is where the generated visitor leaves the namespace ID of the
// part of an admin message being visited.
type namespaceIDKey struct{}

// withNamespaceID returns ctx carrying id, the namespace ID the part of an admin
// message about to be visited names. One message can carry replication tasks
// from many namespaces, so the generated visitor calls this for each part that
// names one, and the payloads below it see that part's ID. The ID is stored as
// found, empty included; making sense of it is left to whoever reads it.
func withNamespaceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, namespaceIDKey{}, id)
}

// namespaceIDFrom returns the namespace ID withNamespaceID put on ctx, and
// whether there was one.
func namespaceIDFrom(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(namespaceIDKey{}).(string)
	return id, ok
}

// visitPayloadInPlace runs the payload visitor over p, a payload held directly by
// a server-internal message, and copies the result over p. Copying rather than
// swapping the pointer means the generated code needs no setters, and it works
// for map values too. proxy.VisitPayloads cannot do this: it does not accept a
// bare Payload.
func visitPayloadInPlace(ctx context.Context, opts AdminVisitOptions, p *common.Payload) error {
	if p == nil {
		return nil
	}

	vctx := &proxy.VisitPayloadsContext{Context: ctx, SinglePayloadRequired: true}
	out, err := opts.PayloadOptions.Visitor(vctx, []*common.Payload{p})
	if err != nil {
		return err
	}
	if len(out) != 1 {
		return fmt.Errorf("payload visitor returned %d payloads for a single payload, want 1", len(out))
	}

	// Resetting p when the visitor handed it back unchanged would wipe it.
	if out[0] != p {
		proto.Reset(p)
		proto.Merge(p, out[0])
	}

	return nil
}

// visitMessage hands a public API message to proxy.VisitPayloads, and to
// proxy.VisitFailures when there is failure work, which already know every
// payload and failure such a message can hold. An unset field arrives as a typed
// nil, which is skipped.
func visitMessage(ctx context.Context, opts AdminVisitOptions, m proto.Message) error {
	if m == nil || !m.ProtoReflect().IsValid() {
		return nil
	}

	return visitPublic(ctx, opts, m)
}

// visitPublic visits m's payloads, and its failures when opts has failure work,
// in the order opts asks for.
func visitPublic(ctx context.Context, opts AdminVisitOptions, m proto.Message) error {
	payloads := func() error { return proxy.VisitPayloads(ctx, m, *opts.PayloadOptions) }
	if opts.FailureOptions == nil {
		return payloads()
	}

	failures := func() error { return proxy.VisitFailures(ctx, m, *opts.FailureOptions) }
	first, second := failures, payloads
	if opts.FailuresAfter {
		first, second = payloads, failures
	}

	if err := first(); err != nil {
		return err
	}

	return second()
}

// visitEventBlob visits the payloads in a blob of serialized history events and
// writes the reserialized events back into the same blob. It always
// reserializes: deciding whether anything changed would mean trusting the
// visitor never to edit a payload in place, and getting that wrong would leave
// plaintext in the blob. A blob that cannot be decoded is an error rather than
// something to pass along unread.
func visitEventBlob(ctx context.Context, opts AdminVisitOptions, blob *common.DataBlob) error {
	if blob == nil || len(blob.GetData()) == 0 {
		return nil
	}

	events, err := serializer.DeserializeEvents(blob)
	if err != nil {
		if !s2scommon.IsInvalidUTF8Error(err) {
			return fmt.Errorf("failed to deserialize history events: %w", err)
		}
		repaired, changed, rerr := tryRepairInvalidUTF8InBlob(blob)
		if rerr != nil {
			return fmt.Errorf("failed to repair invalid utf-8 in history events: %w", rerr)
		}
		// The repair only sees fields the 1.22 protos know about. When it finds
		// nothing to fix it returns no events, and carrying on would write an
		// empty blob over the original.
		if !changed || repaired == nil {
			return fmt.Errorf("failed to deserialize history events: %w", err)
		}
		events = repaired
	}

	if err := visitPublic(ctx, opts, &history.History{Events: events}); err != nil {
		return err
	}

	out, err := serializer.SerializeEvents(events)
	if err != nil {
		return fmt.Errorf("failed to serialize history events: %w", err)
	}

	blob.EncodingType = out.GetEncodingType()
	blob.Data = out.GetData()
	return nil
}

// taskInfoByCategory returns an empty message of the type a serialized history
// task of each persisted category decodes to. The generator walks the same
// types as DecodeTargets.
var taskInfoByCategory = map[int32]func() proto.Message{
	tasks.CategoryIDTransfer:    func() proto.Message { return &persistence.TransferTaskInfo{} },
	tasks.CategoryIDTimer:       func() proto.Message { return &persistence.TimerTaskInfo{} },
	tasks.CategoryIDReplication: func() proto.Message { return &persistence.ReplicationTaskInfo{} },
	tasks.CategoryIDVisibility:  func() proto.Message { return &persistence.VisibilityTaskInfo{} },
	tasks.CategoryIDArchival:    func() proto.Message { return &persistence.ArchivalTaskInfo{} },
	tasks.CategoryIDOutbound:    func() proto.Message { return &persistence.OutboundTaskInfo{} },
}

// visitAddTasksRequestTask decodes the history task in t by its category, visits
// it, and writes the reencoded task back into the same blob. Like
// visitEventBlob, it always reencodes. A task it cannot decode, including one
// of a category registered at runtime, whose type the proxy does not know, is
// reported whole as opaque, so the caller decides whether it may cross.
func visitAddTasksRequestTask(ctx context.Context, opts AdminVisitOptions, path string, t *adminservice.AddTasksRequest_Task) error {
	blob := t.GetBlob()
	if len(blob.GetData()) == 0 {
		return nil
	}

	newInfo, ok := taskInfoByCategory[t.GetCategoryId()]
	if !ok {
		return visitOpaque(opts, path+"/Blob", blob.GetData())
	}

	info := newInfo()
	if err := serialization.Decode(blob, info); err != nil {
		return visitOpaque(opts, path+"/Blob", blob.GetData())
	}

	if err := visitAdminPayloads(ctx, info, opts); err != nil {
		return err
	}

	out, err := serialization.Encode(info)
	if err != nil {
		return fmt.Errorf("%s: failed to serialize history task: %w", path, err)
	}

	blob.EncodingType = out.GetEncodingType()
	blob.Data = out.GetData()
	return nil
}

// visitOpaque reports non-empty data the visitor cannot see inside.
func visitOpaque(opts AdminVisitOptions, path string, data []byte) error {
	if len(data) == 0 {
		return nil
	}

	return opts.Opaque(path, data)
}

// visitStateMachineNode reports the data of n and of every node below it. The
// tree is recursive, which generated code cannot unroll, so it is walked here.
func visitStateMachineNode(ctx context.Context, opts AdminVisitOptions, path string, n *persistence.StateMachineNode) error {
	if n == nil {
		return nil
	}

	if err := visitOpaque(opts, path+"/Data", n.GetData()); err != nil {
		return err
	}

	for typ, machines := range n.GetChildren() {
		for id, child := range machines.GetMachinesById() {
			childPath := fmt.Sprintf("%s/Children[%s]/%s", path, typ, id)
			if err := visitStateMachineNode(ctx, opts, childPath, child); err != nil {
				return err
			}
		}
	}

	return nil
}
