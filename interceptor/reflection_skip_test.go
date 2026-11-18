package interceptor

import (
	"bytes"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/temporalio/s2s-proxy/auth"
)

// proxiedServices are the gRPC services whose requests and responses go through the visitors.
var proxiedServices = []protoreflect.FullName{
	"temporal.api.workflowservice.v1.WorkflowService",
	"temporal.api.operatorservice.v1.OperatorService",
	"temporal.server.api.adminservice.v1.AdminService",
}

// deepMessageTypes are message types that stream replication carries deep inside a response.
// The random builder stops at randomMessageMaxDepth, so a random StreamWorkflowReplicationMessagesResponse
// has no event attributes in its history blobs and no mutable state in its state snapshots.
// A random message of one of these types starts at that depth, so its history blobs have events
// with search attributes and namespace fields, and its mutable state has search attributes.
var deepMessageTypes = []protoreflect.FullName{
	"temporal.server.api.replication.v1.HistoryTaskAttributes",
	"temporal.server.api.replication.v1.BackfillHistoryTaskAttributes",
	"temporal.server.api.replication.v1.VersionedTransitionArtifact",
	"temporal.server.api.persistence.v1.WorkflowMutableState",
	"temporal.server.api.persistence.v1.WorkflowMutableStateMutation",
}

// randomMessagesPerType is the number of random messages for each request and response type.
const randomMessagesPerType = 50

// randomMessagesPerDeepType is the number of random messages for each type in deepMessageTypes.
// It is larger than randomMessagesPerType because few random events hold a search attribute, and
// a message with one random blob that is not a history makes both runs fail, so the check skips it.
const randomMessagesPerDeepType = 500

// skipCheckVisitors are the visitors that the byte slice checks run, each with a matcher that
// translates or allows one of the names in randomStrings.
var skipCheckVisitors = []struct {
	name  string
	visit visitor
	match stringMatcher
}{
	{name: "namespace", visit: visitNamespace, match: createStringMatcher(map[string]string{"orig": "orig.cloud"})},
	{name: "search attribute", visit: visitSearchAttributesWithOneMapping, match: createStringMatcher(map[string]string{"AudienceId": "Keyword01"})},
	{name: "access control", visit: visitNamespace, match: createNamespaceAccessControl(auth.NewAccesControl([]string{"orig"}))},
}

// TestSkippingByteSlicesKeepsVisitorResults checks that skipping byte slices only saves work,
// for random requests and responses of every method in proxiedServices.
func TestSkippingByteSlicesKeepsVisitorResults(t *testing.T) {
	t.Cleanup(func() { walkByteSlices = false })
	logger := log.NewNoopLogger()

	for _, serviceName := range proxiedServices {
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(serviceName)
		require.NoError(t, err)
		methods := desc.(protoreflect.ServiceDescriptor).Methods()
		for i := range methods.Len() {
			method := methods.Get(i)
			for _, msgDesc := range []protoreflect.MessageDescriptor{method.Input(), method.Output()} {
				msgType, err := protoregistry.GlobalTypes.FindMessageByName(msgDesc.FullName())
				require.NoError(t, err)
				for seed := range uint64(randomMessagesPerType) {
					requireSameVisitorResults(t, logger, msgType, seed, "method", method.FullName())
				}
			}
		}
	}
}

// TestSkippingByteSlicesKeepsVisitorResultsInDeepMessages runs the same check on random messages
// of deepMessageTypes. For each type, the namespace and the search attribute visitors must each
// match in at least one message, so that the check covers translation and not only messages
// with nothing to translate.
func TestSkippingByteSlicesKeepsVisitorResultsInDeepMessages(t *testing.T) {
	t.Cleanup(func() { walkByteSlices = false })
	logger := log.NewNoopLogger()

	for _, name := range deepMessageTypes {
		msgType, err := protoregistry.GlobalTypes.FindMessageByName(name)
		require.NoError(t, err)
		matched := map[string]bool{}
		for seed := range uint64(randomMessagesPerDeepType) {
			for _, visitorName := range requireSameVisitorResults(t, logger, msgType, seed) {
				matched[visitorName] = true
			}
		}
		require.True(t, matched["namespace"], "the namespace visitor matched no %s message", name)
		require.True(t, matched["search attribute"], "the search attribute visitor matched no %s message", name)
	}
}

// TestNoBytesFieldHasAVisitorFieldName checks the one case where skipping byte slices could change
// a visitor result. The visitors skip a byte slice before they look at its field name. So a bytes
// field named like a field in namespaceFieldNames, dataBlobFieldNames or searchAttributeFieldNames
// would not get its rule. For example, the search attribute visitor returns an error for a
// SearchAttributes field of an unknown type, but it would skip a bytes field with that name.
// No such field exists, and this test fails if a new proto adds one.
func TestNoBytesFieldHasAVisitorFieldName(t *testing.T) {
	// protoc-gen-go makes the Go field name from the proto field name without the underscores,
	// so the lowercase names of the two are the same.
	lowerGoName := func(field protoreflect.FieldDescriptor) string {
		return strings.ToLower(strings.ReplaceAll(string(field.Name()), "_", ""))
	}
	ruleNames := map[string]bool{}
	for _, names := range []map[string]bool{namespaceFieldNames, dataBlobFieldNames, searchAttributeFieldNames} {
		for name := range names {
			ruleNames[strings.ToLower(name)] = true
		}
	}

	var checked int
	var found []protoreflect.FullName
	protoregistry.GlobalTypes.RangeMessages(func(msgType protoreflect.MessageType) bool {
		desc := msgType.Descriptor()
		if !strings.HasPrefix(string(desc.FullName()), "temporal.") {
			return true
		}
		checked++
		fields := desc.Fields()
		for i := range fields.Len() {
			field := fields.Get(i)
			// A list or a map of bytes is not a byte slice, so the visitors do not skip it.
			if field.Kind() == protoreflect.BytesKind && !field.IsList() && !field.IsMap() && ruleNames[lowerGoName(field)] {
				found = append(found, field.FullName())
			}
		}
		return true
	})
	require.Empty(t, found, "bytes fields with a visitor field name")

	// The check must include the replication types and must map rule names to proto field names.
	require.Greater(t, checked, 1000, "temporal message types checked")
	for _, name := range deepMessageTypes {
		_, err := protoregistry.GlobalTypes.FindMessageByName(name)
		require.NoError(t, err)
	}
	started := (&history.WorkflowExecutionStartedEventAttributes{}).ProtoReflect().Descriptor().Fields()
	require.True(t, ruleNames[lowerGoName(started.ByName("parent_workflow_namespace"))])
	require.True(t, ruleNames[lowerGoName(started.ByName("search_attributes"))])
}

// TestVisitorsTranslateStreamBlobsWithLargePayload checks that both visitors translate each history
// blob field and mutable state field that stream replication carries, when the events and the search
// attributes have large payloads. The payloads must stay the same, and a visitor must not make an
// allocation for each payload byte.
func TestVisitorsTranslateStreamBlobsWithLargePayload(t *testing.T) {
	mapping := createStringMatcher(map[string]string{"orig": "orig.cloud"})
	expected := map[string]*adminservice.StreamWorkflowReplicationMessagesResponse{
		"namespace":        makeStreamResponse(t, "orig.cloud", "orig", largePayloadSize),
		"search attribute": makeStreamResponse(t, "orig", "orig.cloud", largePayloadSize),
	}
	normalizeHistoryBlobs(expected["namespace"].ProtoReflect())
	normalizeHistoryBlobs(expected["search attribute"].ProtoReflect())
	original := makeStreamResponse(t, "orig", "orig", largePayloadSize)
	logger := log.NewTestLogger()

	for _, v := range historyVisitors {
		t.Run(v.name, func(t *testing.T) {
			resp := proto.Clone(original)
			changed, err := v.visit(logger, resp, mapping)
			require.NoError(t, err)
			require.True(t, changed)
			normalizeHistoryBlobs(resp.ProtoReflect())
			// Do not print the response on failure: it holds 2.4 MB of payloads.
			require.True(t, proto.Equal(expected[v.name], resp), "translated response differs from the expected response")

			allocs := testing.AllocsPerRun(3, func() {
				// Translation replaces blobs in the response, so give each run its own response.
				if _, err := v.visit(logger, proto.Clone(original), mapping); err != nil {
					t.Fatal(err)
				}
			})
			// A visitor that walks each byte allocates at least once for each payload byte.
			payloadBytes := streamResponseLargePayloads * largePayloadSize
			require.Less(t, allocs, float64(payloadBytes/16))
		})
	}
}

// streamResponseLargePayloads is the number of payloads of payloadSize bytes in makeStreamResponse:
// 9 history blobs with 4 each, and 2 mutable state search attributes.
const streamResponseLargePayloads = 9*4 + 2

// makeStreamResponse returns a stream replication response with a history blob in each blob field of
// a history task, a backfill task, and a state snapshot task and a state mutation task. Each blob holds
// makeTranslatableEvents. The snapshot and the mutation also have a search attribute in the execution
// info, with a value of payloadSize bytes.
func makeStreamResponse(t *testing.T, namespace, searchAttribute string, payloadSize int) *adminservice.StreamWorkflowReplicationMessagesResponse {
	blob := func() *common.DataBlob {
		blob, err := serializer.SerializeEvents(makeTranslatableEvents(namespace, searchAttribute, payloadSize))
		require.NoError(t, err)
		return blob
	}
	executionInfo := func() *persistence.WorkflowExecutionInfo {
		return &persistence.WorkflowExecutionInfo{SearchAttributes: map[string]*common.Payload{
			searchAttribute: {Data: bytes.Repeat([]byte{0xab}, payloadSize)},
		}}
	}
	return &adminservice.StreamWorkflowReplicationMessagesResponse{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
			Messages: &replicationspb.WorkflowReplicationMessages{
				ReplicationTasks: []*replicationspb.ReplicationTask{
					{Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
						HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
							Events:        blob(),
							NewRunEvents:  blob(),
							EventsBatches: []*common.DataBlob{blob(), blob()},
						},
					}},
					{Attributes: &replicationspb.ReplicationTask_BackfillHistoryTaskAttributes{
						BackfillHistoryTaskAttributes: &replicationspb.BackfillHistoryTaskAttributes{
							EventBatches: []*common.DataBlob{blob()},
							NewRunInfo:   &replicationspb.NewRunInfo{EventBatch: blob()},
						},
					}},
					{Attributes: &replicationspb.ReplicationTask_SyncVersionedTransitionTaskAttributes{
						SyncVersionedTransitionTaskAttributes: &replicationspb.SyncVersionedTransitionTaskAttributes{
							VersionedTransitionArtifact: &replicationspb.VersionedTransitionArtifact{
								StateAttributes: &replicationspb.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes{
									SyncWorkflowStateSnapshotAttributes: &replicationspb.SyncWorkflowStateSnapshotAttributes{
										State: &persistence.WorkflowMutableState{ExecutionInfo: executionInfo()},
									},
								},
								EventBatches: []*common.DataBlob{blob()},
								NewRunInfo:   &replicationspb.NewRunInfo{EventBatch: blob()},
							},
						},
					}},
					{Attributes: &replicationspb.ReplicationTask_SyncVersionedTransitionTaskAttributes{
						SyncVersionedTransitionTaskAttributes: &replicationspb.SyncVersionedTransitionTaskAttributes{
							VersionedTransitionArtifact: &replicationspb.VersionedTransitionArtifact{
								StateAttributes: &replicationspb.VersionedTransitionArtifact_SyncWorkflowStateMutationAttributes{
									SyncWorkflowStateMutationAttributes: &replicationspb.SyncWorkflowStateMutationAttributes{
										StateMutation: &persistence.WorkflowMutableStateMutation{ExecutionInfo: executionInfo()},
									},
								},
								EventBatches: []*common.DataBlob{blob()},
							},
						},
					}},
				},
			},
		},
	}
}

// makeTranslatableEvents returns one event for each kind of place where the visitors translate:
// a started event (parent namespace and search attribute), a search attribute upsert, a child
// workflow start (namespace and search attribute) and an external signal (namespace). It also returns
// an activity event with no namespace field. Four events have an input payload of payloadSize bytes.
func makeTranslatableEvents(namespace, searchAttribute string, payloadSize int) []*history.HistoryEvent {
	input := func() *common.Payloads {
		return &common.Payloads{Payloads: []*common.Payload{{
			Metadata: map[string][]byte{"encoding": []byte("binary/plain")},
			Data:     bytes.Repeat([]byte{0xab}, payloadSize),
		}}}
	}
	searchAttributes := func() *common.SearchAttributes {
		return &common.SearchAttributes{IndexedFields: map[string]*common.Payload{
			searchAttribute: {Data: []byte(`"value"`)},
		}}
	}
	return []*history.HistoryEvent{
		{
			EventId:   1,
			EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
					ParentWorkflowNamespace: namespace,
					Input:                   input(),
					SearchAttributes:        searchAttributes(),
				},
			},
		},
		{
			EventId:   2,
			EventType: enums.EVENT_TYPE_UPSERT_WORKFLOW_SEARCH_ATTRIBUTES,
			Attributes: &history.HistoryEvent_UpsertWorkflowSearchAttributesEventAttributes{
				UpsertWorkflowSearchAttributesEventAttributes: &history.UpsertWorkflowSearchAttributesEventAttributes{
					SearchAttributes: searchAttributes(),
				},
			},
		},
		{
			EventId:   3,
			EventType: enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED,
			Attributes: &history.HistoryEvent_StartChildWorkflowExecutionInitiatedEventAttributes{
				StartChildWorkflowExecutionInitiatedEventAttributes: &history.StartChildWorkflowExecutionInitiatedEventAttributes{
					Namespace:        namespace,
					Input:            input(),
					SearchAttributes: searchAttributes(),
				},
			},
		},
		{
			EventId:   4,
			EventType: enums.EVENT_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION_INITIATED,
			Attributes: &history.HistoryEvent_SignalExternalWorkflowExecutionInitiatedEventAttributes{
				SignalExternalWorkflowExecutionInitiatedEventAttributes: &history.SignalExternalWorkflowExecutionInitiatedEventAttributes{
					Namespace: namespace,
					Input:     input(),
				},
			},
		},
		{
			EventId:   5,
			EventType: enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED,
			Attributes: &history.HistoryEvent_ActivityTaskScheduledEventAttributes{
				ActivityTaskScheduledEventAttributes: &history.ActivityTaskScheduledEventAttributes{
					Input: input(),
				},
			},
		},
	}
}

// requireSameVisitorResults runs each visitor in skipCheckVisitors on one random message of msgType,
// once with the byte slice walk and once without it. Both runs must return the same message and
// the same matched value, or both must return an error. After an error the message is only partly
// translated and its content is not defined, so then it only checks that both fail.
// It returns the names of the visitors that matched without an error.
func requireSameVisitorResults(t *testing.T, logger log.Logger, msgType protoreflect.MessageType, seed uint64, location ...any) []string {
	t.Helper()
	msg := newRandomMessage(msgType, seed)
	var matched []string
	for _, v := range skipCheckVisitors {
		walked := runVisitor(logger, v.visit, v.match, msg, true)
		skipped := runVisitor(logger, v.visit, v.match, msg, false)
		where := append(slices.Clip(location), "type", msgType.Descriptor().FullName(), "visitor", v.name, "seed", seed)
		require.Equal(t, walked.err == nil, skipped.err == nil,
			"only one run failed: walked=%v skipped=%v %v", walked.err, skipped.err, where)
		if walked.err != nil {
			continue
		}
		require.Equal(t, walked.matched, skipped.matched, "matched differs %v", where)
		require.True(t, proto.Equal(walked.msg, skipped.msg), "message differs %v", where)
		if skipped.matched {
			matched = append(matched, v.name)
		}
	}
	return matched
}

type visitorResult struct {
	msg     proto.Message
	matched bool
	err     error
}

// runVisitor runs the visitor on a copy of msg, with or without the byte slice walk.
func runVisitor(logger log.Logger, visit visitor, match stringMatcher, msg proto.Message, walk bool) visitorResult {
	walkByteSlices = walk
	out := proto.Clone(msg)
	matched, err := visit(logger, out, match)
	normalizeHistoryBlobs(out.ProtoReflect())
	return visitorResult{msg: out, matched: matched, err: err}
}

// normalizeHistoryBlobs encodes each data blob that holds a history again with a fixed map order.
// A translated blob is encoded again with random map order, so equal events can give different bytes.
func normalizeHistoryBlobs(m protoreflect.Message) {
	if m.Descriptor().FullName() == dataBlobName {
		dataField := m.Descriptor().Fields().ByName("data")
		var events history.History
		if err := proto.Unmarshal(m.Get(dataField).Bytes(), &events); err != nil {
			return
		}
		normalizeHistoryBlobs(events.ProtoReflect())
		data, err := proto.MarshalOptions{Deterministic: true}.Marshal(&events)
		if err == nil {
			m.Set(dataField, protoreflect.ValueOfBytes(data))
		}
		return
	}
	m.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsMap():
			if field.MapValue().Kind() == protoreflect.MessageKind {
				value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
					normalizeHistoryBlobs(v.Message())
					return true
				})
			}
		case field.IsList():
			if field.Kind() == protoreflect.MessageKind {
				for i := range value.List().Len() {
					normalizeHistoryBlobs(value.List().Get(i).Message())
				}
			}
		case field.Kind() == protoreflect.MessageKind:
			normalizeHistoryBlobs(value.Message())
		}
		return true
	})
}

var dataBlobName = (&common.DataBlob{}).ProtoReflect().Descriptor().FullName()

// randomStrings holds the names that the test matchers translate, and names that they do not.
var randomStrings = []string{"orig", "other", "AudienceId", ""}

// randomMessageMaxDepth limits how deep the builder fills nested messages.
const randomMessageMaxDepth = 5

// randomMessageBuilder fills a message with random values. Most data blobs, and some other byte
// fields, hold a serialized history of random events, so the visitors also translate inside blobs.
type randomMessageBuilder struct {
	rand *rand.Rand
}

func newRandomMessage(msgType protoreflect.MessageType, seed uint64) proto.Message {
	b := &randomMessageBuilder{rand: rand.New(rand.NewPCG(seed, 0))}
	m := msgType.New()
	b.fill(m, 0)
	return m.Interface()
}

func (b *randomMessageBuilder) fill(m protoreflect.Message, depth int) {
	desc := m.Descriptor()
	if desc.FullName() == dataBlobName && b.rand.IntN(10) < 7 {
		m.Set(desc.Fields().ByName("encoding_type"), protoreflect.ValueOfEnum(enums.ENCODING_TYPE_PROTO3.Number()))
		m.Set(desc.Fields().ByName("data"), protoreflect.ValueOfBytes(b.historyBytes(depth)))
		return
	}
	filledOneofs := map[protoreflect.FullName]bool{}
	fields := desc.Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		if oneof := field.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() {
			if filledOneofs[oneof.FullName()] {
				continue
			}
			filledOneofs[oneof.FullName()] = true
			if b.rand.IntN(3) == 0 {
				continue
			}
			field = oneof.Fields().Get(b.rand.IntN(oneof.Fields().Len()))
		} else if b.rand.IntN(4) == 0 {
			continue
		}
		b.setField(m, field, depth)
	}
}

func (b *randomMessageBuilder) setField(m protoreflect.Message, field protoreflect.FieldDescriptor, depth int) {
	switch {
	case field.IsMap():
		if depth >= randomMessageMaxDepth {
			return
		}
		entries := m.Mutable(field).Map()
		for range b.rand.IntN(3) {
			key := b.scalar(field.MapKey(), depth).MapKey()
			if isMessageField(field.MapValue()) {
				value := entries.NewValue()
				b.fill(value.Message(), depth+1)
				entries.Set(key, value)
			} else {
				entries.Set(key, b.scalar(field.MapValue(), depth))
			}
		}
	case field.IsList():
		if isMessageField(field) && depth >= randomMessageMaxDepth {
			return
		}
		list := m.Mutable(field).List()
		for range b.rand.IntN(3) {
			if isMessageField(field) {
				element := list.NewElement()
				b.fill(element.Message(), depth+1)
				list.Append(element)
			} else {
				list.Append(b.scalar(field, depth))
			}
		}
	case isMessageField(field):
		if depth < randomMessageMaxDepth {
			b.fill(m.Mutable(field).Message(), depth+1)
		}
	default:
		m.Set(field, b.scalar(field, depth))
	}
}

func isMessageField(field protoreflect.FieldDescriptor) bool {
	return field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind
}

func (b *randomMessageBuilder) scalar(field protoreflect.FieldDescriptor, depth int) protoreflect.Value {
	switch field.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(b.rand.IntN(2) == 0)
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		return protoreflect.ValueOfEnum(values.Get(b.rand.IntN(values.Len())).Number())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(b.rand.Int32N(1000))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(b.rand.Int64N(100000))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(b.rand.Uint32N(1000))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(b.rand.Uint64N(100000))
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(b.rand.Float32())
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(b.rand.Float64())
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(randomStrings[b.rand.IntN(len(randomStrings))])
	case protoreflect.BytesKind:
		switch b.rand.IntN(3) {
		case 0:
			// Translatable names as raw bytes. The visitors must leave them unchanged.
			return protoreflect.ValueOfBytes([]byte("orig AudienceId"))
		case 1:
			data := make([]byte, b.rand.IntN(64))
			for i := range data {
				data[i] = byte(b.rand.Uint32())
			}
			return protoreflect.ValueOfBytes(data)
		default:
			return protoreflect.ValueOfBytes(b.historyBytes(depth))
		}
	default:
		panic("unhandled field kind " + field.Kind().String())
	}
}

// historyBytes returns a serialized history of one to three random events.
func (b *randomMessageBuilder) historyBytes(depth int) []byte {
	if depth >= randomMessageMaxDepth {
		return nil
	}
	var events history.History
	for range 1 + b.rand.IntN(3) {
		var event history.HistoryEvent
		b.fill(event.ProtoReflect(), depth+1)
		events.Events = append(events.Events, &event)
	}
	data, err := proto.Marshal(&events)
	if err != nil {
		panic(err)
	}
	return data
}
