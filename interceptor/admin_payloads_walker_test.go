package interceptor

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/proxy"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	persistence "go.temporal.io/server/api/persistence/v1"
	replication "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/persistence/serialization"
	"go.temporal.io/server/service/history/tasks"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

const adminPackage protoreflect.FullName = "temporal.server.api.adminservice.v1"

func adminMessageTypes(t *testing.T) []protoreflect.MessageType {
	t.Helper()
	var out []protoreflect.MessageType
	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		if mt.Descriptor().ParentFile().Package() == adminPackage {
			out = append(out, mt)
		}
		return true
	})
	require.NotEmpty(t, out)
	return out
}

func TestVisitAdminPayloadsZeroValues(t *testing.T) {
	for _, mt := range adminMessageTypes(t) {
		t.Run(string(mt.Descriptor().FullName()), func(t *testing.T) {
			var calls atomic.Int32
			require.NoError(t, VisitAdminPayloads(context.Background(), mt.New().Interface(), sealOpts(&calls)))
			require.Zero(t, calls.Load())
		})
	}
}

// leafPath is a sequence of fields from a root message down to a Payload field.
type leafPath []protoreflect.FieldDescriptor

func (p leafPath) String() string {
	parts := make([]string, len(p))
	for i, f := range p {
		parts[i] = string(f.Name())
	}
	return strings.Join(parts, "/")
}

// payloadLeaves lists every path from md to a Payload, without entering bytes,
// Any, or a message type already on the path, and no deeper than maxDepth.
func payloadLeaves(md protoreflect.MessageDescriptor, path leafPath, onPath map[protoreflect.FullName]bool, maxDepth int, out *[]leafPath) {
	if len(path) >= maxDepth {
		return
	}
	onPath[md.FullName()] = true
	defer delete(onPath, md.FullName())

	fields := md.Fields()
	for i := range fields.Len() {
		f := fields.Get(i)
		vf := f
		if f.IsMap() {
			vf = f.MapValue()
		}
		if vf.Kind() != protoreflect.MessageKind {
			continue
		}
		child := vf.Message()
		next := append(append(leafPath{}, path...), f)
		switch {
		case child.FullName() == "temporal.api.common.v1.Payload":
			*out = append(*out, next)
		case child.FullName() == "google.protobuf.Any", onPath[child.FullName()]:
			continue
		default:
			payloadLeaves(child, next, onPath, maxDepth, out)
		}
	}
}

// populate builds a fresh mt with only the leaf at path set, and returns it.
func populate(mt protoreflect.MessageType, path leafPath) protoreflect.Message {
	root := mt.New()
	m := root
	for i, f := range path {
		var child protoreflect.Message
		switch {
		case f.IsMap():
			child = m.Mutable(f).Map().Mutable(mapKey(f.MapKey())).Message()
		case f.IsList():
			child = m.Mutable(f).List().AppendMutable().Message()
		default:
			child = m.Mutable(f).Message()
		}
		if i == len(path)-1 {
			child.Interface().(*common.Payload).Data = []byte("leaf")
		}
		m = child
	}
	return root
}

func mapKey(k protoreflect.FieldDescriptor) protoreflect.MapKey {
	switch k.Kind() {
	case protoreflect.StringKind:
		return protoreflect.ValueOfString("k").MapKey()
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(true).MapKey()
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(1).MapKey()
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(1).MapKey()
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(1).MapKey()
	default:
		return protoreflect.ValueOfUint64(1).MapKey()
	}
}

// TestVisitAdminPayloadsReachesEveryPayload sets one payload at a time, at
// every place the schema allows one, and requires the visitor to see exactly
// that one. Search attributes are included (SkipSearchAttributes is off) so
// they are checked too.
func TestVisitAdminPayloadsReachesEveryPayload(t *testing.T) {
	const maxDepth = 25
	for _, mt := range adminMessageTypes(t) {
		var leaves []leafPath
		payloadLeaves(mt.Descriptor(), nil, map[protoreflect.FullName]bool{}, maxDepth, &leaves)

		for _, leaf := range leaves {
			t.Run(string(mt.Descriptor().Name())+"/"+leaf.String(), func(t *testing.T) {
				var calls atomic.Int32
				opts := AdminVisitOptions{
					Payloads: &proxy.VisitPayloadsOptions{Visitor: testSealer(&calls)},
					Opaque:   func(string, []byte) error { return nil },
				}

				msg := populate(mt, leaf).Interface()
				require.NoError(t, VisitAdminPayloads(context.Background(), msg, opts))
				require.EqualValues(t, 1, calls.Load())
			})
		}
	}
}

func streamResponse(tasks ...*replication.ReplicationTask) *adminservice.StreamWorkflowReplicationMessagesResponse {
	return &adminservice.StreamWorkflowReplicationMessagesResponse{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
			Messages: &replication.WorkflowReplicationMessages{ReplicationTasks: tasks},
		},
	}
}

func snapshotTask(state *persistence.WorkflowMutableState) *replication.ReplicationTask {
	return &replication.ReplicationTask{
		Attributes: &replication.ReplicationTask_SyncVersionedTransitionTaskAttributes{
			SyncVersionedTransitionTaskAttributes: &replication.SyncVersionedTransitionTaskAttributes{
				VersionedTransitionArtifact: &replication.VersionedTransitionArtifact{
					StateAttributes: &replication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes{
						SyncWorkflowStateSnapshotAttributes: &replication.SyncWorkflowStateSnapshotAttributes{State: state},
					},
				},
			},
		},
	}
}

func TestVisitAdminPayloadsStreamHistoryEvents(t *testing.T) {
	var calls atomic.Int32
	blob, err := serializer.SerializeEvents(startedEvents())
	require.NoError(t, err)
	resp := streamResponse(&replication.ReplicationTask{
		Attributes: &replication.ReplicationTask_HistoryTaskAttributes{
			HistoryTaskAttributes: &replication.HistoryTaskAttributes{Events: blob},
		},
	})

	require.NoError(t, VisitAdminPayloads(context.Background(), resp, sealOpts(&calls)))

	events, err := serializer.DeserializeEvents(resp.GetMessages().GetReplicationTasks()[0].GetHistoryTaskAttributes().GetEvents())
	require.NoError(t, err)
	require.Equal(t, "sealed:input", string(events[0].GetWorkflowExecutionStartedEventAttributes().GetInput().GetPayloads()[0].GetData()))
}

func TestVisitAdminPayloadsMutableStateMemoAndSearchAttributes(t *testing.T) {
	var calls atomic.Int32
	resp := streamResponse(snapshotTask(&persistence.WorkflowMutableState{
		ExecutionInfo: &persistence.WorkflowExecutionInfo{
			Memo:             map[string]*common.Payload{"m": {Data: []byte("memo")}},
			SearchAttributes: map[string]*common.Payload{"s": {Data: []byte("sa")}},
		},
	}))

	require.NoError(t, VisitAdminPayloads(context.Background(), resp, sealOpts(&calls)))

	info := resp.GetMessages().GetReplicationTasks()[0].GetSyncVersionedTransitionTaskAttributes().
		GetVersionedTransitionArtifact().GetSyncWorkflowStateSnapshotAttributes().GetState().GetExecutionInfo()
	require.Equal(t, "sealed:memo", string(info.GetMemo()["m"].GetData()))
	require.Equal(t, "sa", string(info.GetSearchAttributes()["s"].GetData()), "search attributes stay readable")
}

func TestVisitAdminPayloadsReportsOpaqueState(t *testing.T) {
	resp := streamResponse(snapshotTask(&persistence.WorkflowMutableState{
		ExecutionInfo: &persistence.WorkflowExecutionInfo{
			SubStateMachinesByType: map[string]*persistence.StateMachineMap{
				"nexus": {MachinesById: map[string]*persistence.StateMachineNode{
					"op": {Children: map[string]*persistence.StateMachineMap{
						"callback": {MachinesById: map[string]*persistence.StateMachineNode{
							"cb": {Data: []byte("deep")},
						}},
					}},
				}},
			},
		},
		ChasmNodes: map[string]*persistence.ChasmNode{
			"root": {Data: &common.DataBlob{Data: []byte("chasm")}},
		},
	}))

	var got []string
	var calls atomic.Int32
	opts := sealOpts(&calls)
	opts.Opaque = func(_ string, data []byte) error {
		got = append(got, string(data))
		return nil
	}

	require.NoError(t, VisitAdminPayloads(context.Background(), resp, opts))
	require.ElementsMatch(t, []string{"deep", "chasm"}, got)
}

func TestVisitAdminPayloadsDecodesHistoryTasks(t *testing.T) {
	encode := func(m proto.Message) *common.DataBlob {
		blob, err := serialization.Encode(m)
		require.NoError(t, err)
		return blob
	}
	archival := &persistence.ArchivalTaskInfo{WorkflowId: "wf"}
	transfer := &persistence.TransferTaskInfo{TaskDetails: &persistence.TransferTaskInfo_ChasmTaskInfo{
		ChasmTaskInfo: &persistence.ChasmTaskInfo{Data: &common.DataBlob{Data: []byte("chasm")}},
	}}
	req := &adminservice.AddTasksRequest{Tasks: []*adminservice.AddTasksRequest_Task{
		{CategoryId: tasks.CategoryIDArchival, Blob: encode(archival)},
		{CategoryId: tasks.CategoryIDTransfer, Blob: encode(transfer)},
		{CategoryId: tasks.CategoryIDTransfer},
	}}

	var got []string
	var calls atomic.Int32
	opts := sealOpts(&calls)
	opts.Opaque = func(_ string, data []byte) error {
		got = append(got, string(data))
		return nil
	}

	require.NoError(t, VisitAdminPayloads(context.Background(), req, opts))
	require.Equal(t, []string{"chasm"}, got, "only the CHASM data is opaque, not the whole task")

	decoded := &persistence.ArchivalTaskInfo{}
	require.NoError(t, serialization.Decode(req.GetTasks()[0].GetBlob(), decoded))
	require.True(t, proto.Equal(archival, decoded))
}

func TestVisitAdminPayloadsIgnoresOtherMessages(t *testing.T) {
	var calls atomic.Int32
	msg := &common.Payloads{Payloads: []*common.Payload{{Data: []byte("x")}}}

	require.NoError(t, VisitAdminPayloads(context.Background(), msg, sealOpts(&calls)))
	require.Zero(t, calls.Load())
	require.Equal(t, "x", string(msg.GetPayloads()[0].GetData()))
}
