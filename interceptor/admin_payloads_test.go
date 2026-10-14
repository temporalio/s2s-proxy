package interceptor

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/proxy"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	persistence "go.temporal.io/server/api/persistence/v1"
	"go.temporal.io/server/common/persistence/serialization"
	"go.temporal.io/server/service/history/tasks"
	"google.golang.org/protobuf/proto"
)

// testSealer stands in for the encryptor's visitor: it returns a new payload
// for every one it is given, with "sealed:" in front of the data, and counts them.
func testSealer(calls *atomic.Int32) func(*proxy.VisitPayloadsContext, []*common.Payload) ([]*common.Payload, error) {
	return func(_ *proxy.VisitPayloadsContext, in []*common.Payload) ([]*common.Payload, error) {
		out := make([]*common.Payload, len(in))
		for i, p := range in {
			calls.Add(1)
			out[i] = &common.Payload{
				Metadata: map[string][]byte{"encoding": []byte("test/sealed")},
				Data:     append([]byte("sealed:"), p.GetData()...),
			}
		}
		return out, nil
	}
}

func sealOpts(calls *atomic.Int32) AdminVisitOptions {
	return AdminVisitOptions{
		Payloads: &proxy.VisitPayloadsOptions{Visitor: testSealer(calls), SkipSearchAttributes: true},
		Opaque:   func(string, []byte) error { return nil },
	}
}

func TestVisitPayloadInPlace(t *testing.T) {
	t.Run("replaces contents behind the same pointer", func(t *testing.T) {
		var calls atomic.Int32
		p := &common.Payload{Data: []byte("x")}
		held := p

		require.NoError(t, visitPayloadInPlace(context.Background(), sealOpts(&calls), p))
		require.Equal(t, "sealed:x", string(held.GetData()))
		require.Equal(t, "test/sealed", string(held.GetMetadata()["encoding"]))
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("visitor returning the same pointer leaves it intact", func(t *testing.T) {
		opts := AdminVisitOptions{Payloads: &proxy.VisitPayloadsOptions{
			Visitor: func(_ *proxy.VisitPayloadsContext, in []*common.Payload) ([]*common.Payload, error) { return in, nil },
		}}
		p := &common.Payload{Data: []byte("x")}

		require.NoError(t, visitPayloadInPlace(context.Background(), opts, p))
		require.Equal(t, "x", string(p.GetData()))
	})

	t.Run("wrong payload count is an error", func(t *testing.T) {
		opts := AdminVisitOptions{Payloads: &proxy.VisitPayloadsOptions{
			Visitor: func(*proxy.VisitPayloadsContext, []*common.Payload) ([]*common.Payload, error) { return nil, nil },
		}}

		err := visitPayloadInPlace(context.Background(), opts, &common.Payload{Data: []byte("x")})
		require.ErrorContains(t, err, "returned 0 payloads for a single payload")
	})

	t.Run("nil is a no-op", func(t *testing.T) {
		var calls atomic.Int32
		require.NoError(t, visitPayloadInPlace(context.Background(), sealOpts(&calls), nil))
		require.Zero(t, calls.Load())
	})
}

func TestVisitMessage(t *testing.T) {
	t.Run("delegates to proxy.VisitPayloads", func(t *testing.T) {
		var calls atomic.Int32
		f := &failure.Failure{EncodedAttributes: &common.Payload{Data: []byte("x")}}

		require.NoError(t, visitMessage(context.Background(), sealOpts(&calls), f))
		require.Equal(t, "sealed:x", string(f.GetEncodedAttributes().GetData()))
	})

	t.Run("typed nil is a no-op", func(t *testing.T) {
		var calls atomic.Int32
		var f *failure.Failure

		require.NoError(t, visitMessage(context.Background(), sealOpts(&calls), f))
		require.Zero(t, calls.Load())
	})
}

func startedEvents() []*history.HistoryEvent {
	return []*history.HistoryEvent{{
		EventId:   1,
		EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
		Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
				Input: &common.Payloads{Payloads: []*common.Payload{{Data: []byte("input")}}},
				SearchAttributes: &common.SearchAttributes{IndexedFields: map[string]*common.Payload{
					"CustomKeyword": {Data: []byte("sa")},
				}},
			},
		},
	}}
}

func TestVisitEventBlob(t *testing.T) {
	t.Run("seals payloads inside the blob and keeps the rest", func(t *testing.T) {
		var calls atomic.Int32
		blob, err := serializer.SerializeEvents(startedEvents())
		require.NoError(t, err)

		require.NoError(t, visitEventBlob(context.Background(), sealOpts(&calls), blob))

		events, err := serializer.DeserializeEvents(blob)
		require.NoError(t, err)
		require.Len(t, events, 1)
		attrs := events[0].GetWorkflowExecutionStartedEventAttributes()
		require.EqualValues(t, 1, events[0].GetEventId())
		require.Equal(t, "sealed:input", string(attrs.GetInput().GetPayloads()[0].GetData()))
		require.Equal(t, "sa", string(attrs.GetSearchAttributes().GetIndexedFields()["CustomKeyword"].GetData()))
		require.EqualValues(t, 1, calls.Load())
	})

	t.Run("empty blob is a no-op", func(t *testing.T) {
		var calls atomic.Int32
		require.NoError(t, visitEventBlob(context.Background(), sealOpts(&calls), &common.DataBlob{}))
		require.NoError(t, visitEventBlob(context.Background(), sealOpts(&calls), nil))
		require.Zero(t, calls.Load())
	})

	t.Run("undecodable blob is an error, never passed through", func(t *testing.T) {
		var calls atomic.Int32
		blob := &common.DataBlob{EncodingType: enums.ENCODING_TYPE_PROTO3, Data: []byte{0xff, 0xff, 0xff}}

		err := visitEventBlob(context.Background(), sealOpts(&calls), blob)
		require.ErrorContains(t, err, "failed to deserialize history events")
	})
}

func TestVisitOpaque(t *testing.T) {
	var got []string
	opts := AdminVisitOptions{Opaque: func(path string, _ []byte) error {
		got = append(got, path)
		return nil
	}}

	require.NoError(t, visitOpaque(opts, "A/Data", nil))
	require.NoError(t, visitOpaque(opts, "B/Data", []byte("x")))
	require.Equal(t, []string{"B/Data"}, got)
}

func TestVisitStateMachineNode(t *testing.T) {
	nested := &persistence.StateMachineNode{
		Children: map[string]*persistence.StateMachineMap{
			"nexus": {MachinesById: map[string]*persistence.StateMachineNode{
				"op": {Children: map[string]*persistence.StateMachineMap{
					"callback": {MachinesById: map[string]*persistence.StateMachineNode{
						"cb": {Data: []byte("deep")},
					}},
				}},
			}},
		},
	}

	t.Run("reaches data in nested children", func(t *testing.T) {
		var got []string
		opts := AdminVisitOptions{Opaque: func(path string, data []byte) error {
			got = append(got, path+"="+string(data))
			return nil
		}}

		require.NoError(t, visitStateMachineNode(context.Background(), opts, "Root", nested))
		require.Equal(t, []string{"Root/Children[nexus]/op/Children[callback]/cb/Data=deep"}, got)
	})

	t.Run("error stops the visit", func(t *testing.T) {
		opts := AdminVisitOptions{Opaque: func(string, []byte) error { return errors.New("refused") }}
		require.ErrorContains(t, visitStateMachineNode(context.Background(), opts, "Root", nested), "refused")
	})

	t.Run("nil node is a no-op", func(t *testing.T) {
		opts := AdminVisitOptions{Opaque: func(string, []byte) error { return errors.New("must not be called") }}
		require.NoError(t, visitStateMachineNode(context.Background(), opts, "Root", nil))
	})
}

func TestVisitAdminPayloadsRequiresOptions(t *testing.T) {
	var calls atomic.Int32
	ok := sealOpts(&calls)

	noVisitor := ok
	noVisitor.Payloads = nil
	require.ErrorContains(t, VisitAdminPayloads(context.Background(), &adminservice.ReapplyEventsRequest{}, noVisitor), "requires a payload visitor")

	noOpaque := ok
	noOpaque.Opaque = nil
	require.ErrorContains(t, VisitAdminPayloads(context.Background(), &adminservice.ReapplyEventsRequest{}, noOpaque), "requires an opaque data handler")
}

// Invalid UTF-8 in a field the 1.22 protos do not know (links) cannot be
// repaired. The blob must be refused, not emptied.
func TestVisitEventBlobUnrepairableUTF8(t *testing.T) {
	var calls atomic.Int32
	events := startedEvents()
	events[0].Links = []*common.Link{{Variant: &common.Link_WorkflowEvent_{
		WorkflowEvent: &common.Link_WorkflowEvent{Namespace: "MARKERNS"},
	}}}
	blob, err := serializer.SerializeEvents(events)
	require.NoError(t, err)
	blob.Data = bytes.Replace(blob.Data, []byte("MARKERNS"), []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, 1)
	original := bytes.Clone(blob.Data)

	err = visitEventBlob(context.Background(), sealOpts(&calls), blob)
	require.ErrorContains(t, err, "history events")
	require.Equal(t, original, blob.Data, "a refused blob is left as it was")
}

func TestVisitAddTasksRequestTask(t *testing.T) {
	chasm := &persistence.ChasmTaskInfo{Data: &common.DataBlob{Data: []byte("component")}}
	cases := map[int32]proto.Message{
		tasks.CategoryIDTransfer:   &persistence.TransferTaskInfo{TaskDetails: &persistence.TransferTaskInfo_ChasmTaskInfo{ChasmTaskInfo: chasm}},
		tasks.CategoryIDTimer:      &persistence.TimerTaskInfo{TaskDetails: &persistence.TimerTaskInfo_ChasmTaskInfo{ChasmTaskInfo: chasm}},
		tasks.CategoryIDVisibility: &persistence.VisibilityTaskInfo{TaskDetails: &persistence.VisibilityTaskInfo_ChasmTaskInfo{ChasmTaskInfo: chasm}},
		tasks.CategoryIDOutbound:   &persistence.OutboundTaskInfo{TaskDetails: &persistence.OutboundTaskInfo_ChasmTaskInfo{ChasmTaskInfo: chasm}},
	}

	for category, info := range cases {
		t.Run(string(info.ProtoReflect().Descriptor().Name()), func(t *testing.T) {
			blob, err := serialization.Encode(info)
			require.NoError(t, err)
			task := &adminservice.AddTasksRequest_Task{CategoryId: category, Blob: blob}

			var got []string
			opts := AdminVisitOptions{Opaque: func(path string, data []byte) error {
				got = append(got, path+"="+string(data))
				return nil
			}}

			require.NoError(t, visitAddTasksRequestTask(context.Background(), opts, "Task", task))
			require.Len(t, got, 1)
			require.Contains(t, got[0], "ChasmTaskInfo/Data=component")

			decoded := info.ProtoReflect().New().Interface()
			require.NoError(t, serialization.Decode(task.GetBlob(), decoded))
			require.True(t, proto.Equal(info, decoded))
		})
	}
}

// A task the proxy cannot decode is opaque, so the caller decides whether it
// may cross unsealed.
func TestVisitAddTasksRequestTaskUndecodable(t *testing.T) {
	cases := map[string]*adminservice.AddTasksRequest_Task{
		"unknown category": {CategoryId: tasks.CategoryIDMemoryTimer, Blob: &common.DataBlob{
			EncodingType: enums.ENCODING_TYPE_PROTO3,
			Data:         []byte{0x08, 0x01},
		}},
		"undecodable blob": {CategoryId: tasks.CategoryIDTransfer, Blob: &common.DataBlob{
			EncodingType: enums.ENCODING_TYPE_PROTO3,
			Data:         []byte{0xff},
		}},
	}

	for name, task := range cases {
		t.Run(name, func(t *testing.T) {
			want := proto.Clone(task)

			var got []string
			opts := AdminVisitOptions{Opaque: func(path string, data []byte) error {
				got = append(got, path+"="+string(data))
				return nil
			}}
			require.NoError(t, visitAddTasksRequestTask(context.Background(), opts, "Task", task))
			require.Equal(t, []string{"Task/Blob=" + string(task.GetBlob().GetData())}, got)
			require.True(t, proto.Equal(want, task), "an opaque blob is left as it is")

			opts.Opaque = func(string, []byte) error { return errors.New("refused") }
			require.ErrorContains(t, visitAddTasksRequestTask(context.Background(), opts, "Task", task), "refused")
		})
	}
}
