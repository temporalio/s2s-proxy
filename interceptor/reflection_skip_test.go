package interceptor

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/fnv"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	namespacepb "go.temporal.io/api/namespace/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/temporalio/s2s-proxy/auth"
)

// proxiedServices are the gRPC services whose requests and responses go through the visitors.
var proxiedServices = []protoreflect.FullName{
	"temporal.api.workflowservice.v1.WorkflowService",
	"temporal.api.operatorservice.v1.OperatorService",
	"temporal.server.api.adminservice.v1.AdminService",
}

// knownDeepFields are fields that the visitors translate, which stream replication carries deep
// inside a response. The builder stops at testMessageMaxDepth, so a test
// StreamWorkflowReplicationMessagesResponse has no event attributes in its history blobs and no
// mutable state in its state snapshots. reachableMessageTypes finds the types that hold these
// fields, so the test builds them as messages of their own. This list only checks that it does.
var knownDeepFields = []protoreflect.FullName{
	"temporal.server.api.replication.v1.HistoryTaskAttributes.events",
	"temporal.server.api.replication.v1.HistoryTaskAttributes.new_run_events",
	"temporal.server.api.replication.v1.HistoryTaskAttributes.events_batches",
	"temporal.server.api.replication.v1.BackfillHistoryTaskAttributes.event_batches",
	"temporal.server.api.replication.v1.NewRunInfo.event_batch",
	"temporal.server.api.replication.v1.VersionedTransitionArtifact.event_batches",
	"temporal.server.api.persistence.v1.WorkflowExecutionInfo.search_attributes",
}

// visitorRule is a rule that the visitors apply to a field by its name.
type visitorRule string

const (
	namespaceRule       visitorRule = "namespace"
	dataBlobRule        visitorRule = "data blob"
	searchAttributeRule visitorRule = "search attribute"
)

var (
	payloadName          = (&common.Payload{}).ProtoReflect().Descriptor().FullName()
	searchAttributesName = (&common.SearchAttributes{}).ProtoReflect().Descriptor().FullName()
	namespaceInfoName    = (&namespacepb.NamespaceInfo{}).ProtoReflect().Descriptor().FullName()
	historyName          = (&history.History{}).ProtoReflect().Descriptor().FullName()

	lowerNamespaceFieldNames       = lowerNames(namespaceFieldNames)
	lowerDataBlobFieldNames        = lowerNames(dataBlobFieldNames)
	lowerSearchAttributeFieldNames = lowerNames(searchAttributeFieldNames)
)

// lowerGoName returns the Go name of field in lowercase. protoc-gen-go makes the Go field name from
// the proto field name without the underscores, so the lowercase names of the two are the same.
func lowerGoName(field protoreflect.FieldDescriptor) string {
	return strings.ToLower(strings.ReplaceAll(string(field.Name()), "_", ""))
}

func lowerNames(names map[string]bool) map[string]bool {
	out := make(map[string]bool, len(names))
	for name := range names {
		out[strings.ToLower(name)] = true
	}
	return out
}

// visitorRuleOf returns the rule that the visitors apply to field, and whether they can translate
// the field. The namespace visitor finds a NamespaceInfo or a History by its type, in any field.
// The other rules go by the field name. A field with a rule name but another type, such as a
// SearchAttributes list of strings, returns false: the visitors ignore it or return an error for it.
func visitorRuleOf(field protoreflect.FieldDescriptor) (visitorRule, bool) {
	isMessage := func(f protoreflect.FieldDescriptor, name protoreflect.FullName) bool {
		return f.Kind() == protoreflect.MessageKind && f.Message().FullName() == name
	}
	valueField := field
	if field.IsMap() {
		valueField = field.MapValue()
	}
	if isMessage(valueField, namespaceInfoName) || isMessage(valueField, historyName) {
		return namespaceRule, true
	}
	name := lowerGoName(field)
	switch {
	case lowerNamespaceFieldNames[name]:
		return namespaceRule, field.Kind() == protoreflect.StringKind && !field.IsList() && !field.IsMap()
	case lowerDataBlobFieldNames[name]:
		return dataBlobRule, !field.IsMap() && isMessage(field, dataBlobName)
	case lowerSearchAttributeFieldNames[name]:
		if field.IsMap() {
			return searchAttributeRule, isMessage(field.MapValue(), payloadName)
		}
		return searchAttributeRule, !field.IsList() && isMessage(field, searchAttributesName)
	}
	return "", false
}

// reachableMessageTypes returns every message type that a proxied request or response, or a
// history blob, can hold, at any depth, including the requests and responses themselves.
func reachableMessageTypes(t *testing.T) []protoreflect.MessageType {
	t.Helper()
	var types []protoreflect.MessageType
	seen := map[protoreflect.FullName]bool{}
	var walk func(desc protoreflect.MessageDescriptor)
	walk = func(desc protoreflect.MessageDescriptor) {
		if seen[desc.FullName()] {
			return
		}
		seen[desc.FullName()] = true
		// A map entry is part of its map field, not a message of its own.
		if !desc.IsMapEntry() {
			msgType, err := protoregistry.GlobalTypes.FindMessageByName(desc.FullName())
			require.NoError(t, err)
			types = append(types, msgType)
		}
		fields := desc.Fields()
		for i := range fields.Len() {
			field := fields.Get(i)
			if field.IsMap() {
				field = field.MapValue()
			}
			if field.Kind() == protoreflect.MessageKind {
				walk(field.Message())
			}
		}
	}
	for _, msgType := range proxiedMessageTypes(t) {
		walk(msgType.Descriptor())
	}
	// A history blob holds a History, which a proxied message does not always reach as a field.
	walk((&history.History{}).ProtoReflect().Descriptor())
	return types
}

// visitorRules returns the rules that the visitors apply to the fields of desc, and those fields.
func visitorRules(desc protoreflect.MessageDescriptor) (map[visitorRule]bool, []protoreflect.FieldDescriptor) {
	rules := map[visitorRule]bool{}
	var ruleFields []protoreflect.FieldDescriptor
	fields := desc.Fields()
	for i := range fields.Len() {
		if rule, ok := visitorRuleOf(fields.Get(i)); ok {
			rules[rule] = true
			ruleFields = append(ruleFields, fields.Get(i))
		}
	}
	return rules, ruleFields
}

// proxiedMessageTypes returns the request and response types of every method in proxiedServices.
func proxiedMessageTypes(t *testing.T) []protoreflect.MessageType {
	t.Helper()
	var out []protoreflect.MessageType
	seen := map[protoreflect.FullName]bool{}
	for _, serviceName := range proxiedServices {
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(serviceName)
		require.NoError(t, err)
		methods := desc.(protoreflect.ServiceDescriptor).Methods()
		for i := range methods.Len() {
			for _, msgDesc := range []protoreflect.MessageDescriptor{methods.Get(i).Input(), methods.Get(i).Output()} {
				if seen[msgDesc.FullName()] {
					continue
				}
				seen[msgDesc.FullName()] = true
				msgType, err := protoregistry.GlobalTypes.FindMessageByName(msgDesc.FullName())
				require.NoError(t, err)
				out = append(out, msgType)
			}
		}
	}
	return out
}

// skipCheckVisitors are the visitors that the byte slice checks run, each with a matcher that
// translates or allows one of the names in testStrings.
var skipCheckVisitors = []struct {
	name  string
	visit visitor
	match stringMatcher
}{
	{name: "namespace", visit: visitNamespace, match: createStringMatcher(map[string]string{"orig": "orig.cloud"})},
	{name: "search attribute", visit: visitSearchAttributes, match: createStringMatcher(map[string]string{"AudienceId": "Keyword01"})},
	{name: "access control", visit: visitNamespace, match: createNamespaceAccessControl(auth.NewAccesControl([]string{"orig"}))},
}

// TestSkippingByteSlicesKeepsVisitorResults checks that skipping byte slices only saves work, for
// requests and responses of every method in proxiedServices. Each message starts at the request or
// response, so it also checks what the visitors do with the whole message, such as skipping some
// responses. It builds variantsFor messages of each type.
func TestSkippingByteSlicesKeepsVisitorResults(t *testing.T) {
	t.Cleanup(func() { walkByteSlices = false })
	logger := log.NewNoopLogger()

	cov := newTestCoverage()
	for _, msgType := range proxiedMessageTypes(t) {
		for variant := range variantsFor(msgType.Descriptor()) {
			requireSameVisitorResults(t, logger, msgType, variant, cov)
		}
	}
	// Report the coverage of every reachable type, to compare with the test that builds each of them.
	t.Logf("coverage: %v", cov.summary(reachableMessageTypes(t)))
}

// TestSkippingByteSlicesKeepsVisitorResultsInEveryMessageType runs the same check on messages of
// every type from reachableMessageTypes. A visitor decides what to do with a value from the value
// type and the name of the field that holds it. So building each type as a message of its own, in
// variantsFor variants, gives every field of every type each kind of value: each of testStrings,
// each oneof field, and history blobs and other bytes. This also covers the types that stream
// replication carries deeper than a request or response reaches.
//
// It also checks that the comparison covers translation, and not only messages with nothing to
// translate:
//   - for each type, each visitor that one of its fields has a rule for must match in at least one
//     message, and
//   - each field that the visitors translate must hold a value that a test matcher translates in at
//     least one message.
func TestSkippingByteSlicesKeepsVisitorResultsInEveryMessageType(t *testing.T) {
	t.Cleanup(func() { walkByteSlices = false })
	logger := log.NewNoopLogger()
	types := reachableMessageTypes(t)

	var ruleFields []protoreflect.FieldDescriptor
	cov := newTestCoverage()
	for _, msgType := range types {
		desc := msgType.Descriptor()
		rules, fields := visitorRules(desc)
		ruleFields = append(ruleFields, fields...)

		matched := map[string]bool{}
		for variant := range variantsFor(desc) {
			for _, visitorName := range requireSameVisitorResults(t, logger, msgType, variant, cov) {
				matched[visitorName] = true
			}
		}
		if rules[namespaceRule] || rules[dataBlobRule] {
			require.True(t, matched["namespace"], "the namespace visitor matched no %s message", desc.FullName())
		}
		if rules[searchAttributeRule] || rules[dataBlobRule] {
			require.True(t, matched["search attribute"], "the search attribute visitor matched no %s message", desc.FullName())
		}
	}

	for _, field := range ruleFields {
		require.True(t, cov.translatable[field.FullName()], "no message set %s to a value that a test matcher translates", field.FullName())
	}

	summary := cov.summary(types)
	t.Logf("coverage: %v", summary)
	// variantsFor gives each type a variant for each choice of each of its oneofs, and for each test
	// string in each of its string fields that are not lists, maps or in a oneof.
	require.Equal(t, summary.oneofChoices, summary.oneofChoicesMade, "oneof choices made")
	require.Equal(t, summary.stringFields, summary.stringFieldsWithEveryValue, "string fields with every test string")

	// The types must include the stream replication types, and the rules must find their fields.
	require.Greater(t, len(types), 500, "message types checked")
	ruleFieldNames := map[protoreflect.FullName]bool{}
	for _, field := range ruleFields {
		ruleFieldNames[field.FullName()] = true
	}
	for _, name := range knownDeepFields {
		require.True(t, ruleFieldNames[name], "no message type has the field %s", name)
	}
}

// TestNoBytesFieldHasAVisitorFieldName checks the one case where skipping byte slices could change
// a visitor result. The visitors skip a byte slice before they look at its field name. So a bytes
// field named like a field in namespaceFieldNames, dataBlobFieldNames or searchAttributeFieldNames
// would not get its rule. For example, the search attribute visitor returns an error for a
// SearchAttributes field of an unknown type, but it would skip a bytes field with that name.
// No such field exists, and this test fails if a new proto adds one.
func TestNoBytesFieldHasAVisitorFieldName(t *testing.T) {
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
	for _, name := range knownDeepFields {
		_, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
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

// requireSameVisitorResults runs each visitor in skipCheckVisitors on test message number variant of
// msgType, once with the byte slice walk and once without it. Both runs must return the same message
// and the same matched value, or both must return an error. After an error the message is only partly
// translated and its content is not defined, so then it only checks that both fail.
// It adds what the message holds to cov, and returns the names of the visitors that matched
// without an error.
func requireSameVisitorResults(t *testing.T, logger log.Logger, msgType protoreflect.MessageType, variant int, cov *testCoverage) []string {
	t.Helper()
	msg := newTestMessage(msgType, variant, cov)
	var matched []string
	for _, v := range skipCheckVisitors {
		walked := runVisitor(logger, v.visit, v.match, msg, true)
		skipped := runVisitor(logger, v.visit, v.match, msg, false)
		where := []any{"type", msgType.Descriptor().FullName(), "visitor", v.name, "variant", variant}
		require.Equal(t, walked.err == nil, skipped.err == nil,
			"only one run failed: walked=%v skipped=%v %v", walked.err, skipped.err, where)
		if walked.err != nil {
			continue
		}
		require.Equal(t, walked.matched, skipped.matched, "matched differs %v", where)
		if !proto.Equal(walked.msg, skipped.msg) {
			require.Fail(t, "message differs", "%v\n-walked +skipped:\n%s",
				where, cmp.Diff(walked.msg, skipped.msg, protocmp.Transform(), historyBlobsAsEvents))
		}
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

// historyBlobsAsEvents makes cmp.Diff with protocmp.Transform compare the data of a DataBlob as the
// history it holds, so that a difference in a blob shows the event field that differs, not only bytes.
// Data that is not a history stays bytes.
var historyBlobsAsEvents = cmp.FilterPath(func(path cmp.Path) bool {
	if len(path) < 3 {
		return false
	}
	// protocmp.Transform turns a message into a map from field name to value, so the data of a blob
	// is the []byte value under the key "data" of a map that is a DataBlob.
	if _, ok := path.Index(-1).(cmp.TypeAssertion); !ok {
		return false
	}
	key, ok := path.Index(-2).(cmp.MapIndex)
	if !ok || key.Key().String() != "data" {
		return false
	}
	parent, _ := path.Index(-3).Values()
	if !parent.IsValid() || !parent.CanInterface() {
		return false
	}
	blob, ok := parent.Interface().(protocmp.Message)
	return ok && blob.Descriptor().FullName() == dataBlobName
}, cmp.Transformer("historyBlob", func(data []byte) any {
	var events history.History
	if err := proto.Unmarshal(data, &events); err != nil {
		return data
	}
	return &events
}))

// testStrings holds the names that the test matchers translate, and names that they do not.
var testStrings = []string{"orig", "other", "AudienceId", ""}

// testMessageMaxDepth limits how deep the builder fills nested messages. Every reachable type is
// also built as a message of its own, so deeper fields are still checked.
const testMessageMaxDepth = 5

// variantsFor returns the number of test messages to build for a type: enough that each string
// field holds each of testStrings, and each oneof is unset or holds each of its fields, in at
// least one message.
func variantsFor(desc protoreflect.MessageDescriptor) int {
	variants := len(testStrings)
	oneofs := desc.Oneofs()
	for i := range oneofs.Len() {
		if oneof := oneofs.Get(i); !oneof.IsSynthetic() {
			variants = max(variants, oneof.Fields().Len()+1)
		}
	}
	return variants
}

// messageBuilder fills a message with values that depend only on the variant number and on where
// each value is, so the same variant always gives the same message. A value at field index i of
// a message gets the number c = variant + offset + i, and the builder picks a value by c modulo
// the number of choices. In the variants from variantsFor, each field of the top message so gets
// each choice. The offset makes nested messages, list elements and map entries differ from each
// other. Most data blobs, and some other byte fields, hold a serialized history with a translatable
// event, so the visitors also translate inside blobs.
type messageBuilder struct {
	variant int
	// cov collects what the builder set.
	cov *testCoverage
}

// newTestMessage returns test message number variant of msgType, and adds what it set to cov.
func newTestMessage(msgType protoreflect.MessageType, variant int, cov *testCoverage) proto.Message {
	b := &messageBuilder{variant: variant, cov: cov}
	m := msgType.New()
	b.fill(m, 0, 0)
	cov.messages++
	cov.shapes[messageShape(m)] = true
	return m.Interface()
}

// testCoverage collects what the test messages hold, to check and to report how much they cover.
type testCoverage struct {
	// messages is the number of test messages built.
	messages int
	// shapes holds the shape of each test message from messageShape.
	shapes map[uint64]bool
	// oneofChoices holds each oneof choice that a message made: a oneof field, or the oneof unset.
	oneofChoices map[string]bool
	// stringValues holds, for each string field that is not a list, a map or in a oneof, the
	// test strings that it held.
	stringValues map[protoreflect.FullName]map[string]bool
	// translatable holds each field with a visitor rule that held a value that a test matcher translates.
	translatable map[protoreflect.FullName]bool
}

func newTestCoverage() *testCoverage {
	return &testCoverage{
		shapes:       map[uint64]bool{},
		oneofChoices: map[string]bool{},
		stringValues: map[protoreflect.FullName]map[string]bool{},
		translatable: map[protoreflect.FullName]bool{},
	}
}

// oneofChoiceKey names a oneof choice: field is the oneof field that is set, or nil for unset.
func oneofChoiceKey(oneof protoreflect.OneofDescriptor, field protoreflect.FieldDescriptor) string {
	if field == nil {
		return string(oneof.FullName()) + " unset"
	}
	return string(field.FullName())
}

// coverageSummary is how much of a set of message types the test messages cover.
type coverageSummary struct {
	messages int
	types    int
	// shapes is the number of distinct message shapes from messageShape.
	shapes int
	// oneofChoicesMade of oneofChoices: each oneof field, and each oneof unset.
	oneofChoicesMade, oneofChoices int
	// stringFieldsWithEveryValue of stringFields: the string fields that are not lists, maps or in a
	// oneof, and those that held each of testStrings.
	stringFieldsWithEveryValue, stringFields int
	// translatedFieldsFilled of translatedFields: the fields with a visitor rule, and those that held
	// a value that a test matcher translates.
	translatedFieldsFilled, translatedFields int
}

func (s coverageSummary) String() string {
	return fmt.Sprintf("%d messages of %d types, %d distinct shapes; oneof choices %d/%d; "+
		"string fields with every test string %d/%d; translated fields with a translatable value %d/%d",
		s.messages, s.types, s.shapes, s.oneofChoicesMade, s.oneofChoices,
		s.stringFieldsWithEveryValue, s.stringFields, s.translatedFieldsFilled, s.translatedFields)
}

// summary returns how much of types the test messages cover. A choice or a field counts as covered
// if any test message had it, at any depth.
func (cov *testCoverage) summary(types []protoreflect.MessageType) coverageSummary {
	s := coverageSummary{messages: cov.messages, types: len(types), shapes: len(cov.shapes)}
	for _, msgType := range types {
		desc := msgType.Descriptor()
		oneofs := desc.Oneofs()
		for i := range oneofs.Len() {
			oneof := oneofs.Get(i)
			if oneof.IsSynthetic() {
				continue
			}
			s.oneofChoices += oneof.Fields().Len() + 1
			if cov.oneofChoices[oneofChoiceKey(oneof, nil)] {
				s.oneofChoicesMade++
			}
			for j := range oneof.Fields().Len() {
				if cov.oneofChoices[oneofChoiceKey(oneof, oneof.Fields().Get(j))] {
					s.oneofChoicesMade++
				}
			}
		}
		fields := desc.Fields()
		for i := range fields.Len() {
			field := fields.Get(i)
			if isPlainStringField(field) {
				s.stringFields++
				if len(cov.stringValues[field.FullName()]) == len(testStrings) {
					s.stringFieldsWithEveryValue++
				}
			}
			if _, ok := visitorRuleOf(field); ok {
				s.translatedFields++
				if cov.translatable[field.FullName()] {
					s.translatedFieldsFilled++
				}
			}
		}
	}
	return s
}

// isPlainStringField reports whether field is a string field that is not a list, a map or in a oneof.
func isPlainStringField(field protoreflect.FieldDescriptor) bool {
	oneof := field.ContainingOneof()
	return field.Kind() == protoreflect.StringKind && !field.IsList() && !field.IsMap() &&
		(oneof == nil || oneof.IsSynthetic())
}

// messageShape returns a hash of which fields m sets, at every depth, without their values. Two
// messages have the same shape if they set the same fields, with the same number of list elements
// and map entries, and their nested messages have the same shapes.
func messageShape(m protoreflect.Message) uint64 {
	h := fnv.New64a()
	writeShape(h, m)
	return h.Sum64()
}

func writeShape(h hash.Hash64, m protoreflect.Message) {
	writeInt := func(n int) { _ = binary.Write(h, binary.LittleEndian, int64(n)) }
	_, _ = h.Write([]byte(m.Descriptor().FullName()))
	fields := m.Descriptor().Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		if !m.Has(field) {
			continue
		}
		writeInt(int(field.Number()))
		value := m.Get(field)
		switch {
		case field.IsList():
			writeInt(value.List().Len())
			if isMessageField(field) {
				for j := range value.List().Len() {
					writeShape(h, value.List().Get(j).Message())
				}
			}
		case field.IsMap():
			writeInt(value.Map().Len())
			if isMessageField(field.MapValue()) {
				// Map order is random, so write the entry shapes in sorted order.
				var entries []uint64
				value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
					entries = append(entries, messageShape(v.Message()))
					return true
				})
				slices.Sort(entries)
				for _, entry := range entries {
					writeInt(int(entry))
				}
			}
		case isMessageField(field):
			writeShape(h, value.Message())
		}
	}
}

// recordTranslatable adds field to b.cov.translatable if field has a visitor rule and m holds a
// value in it that a test matcher translates: the namespace "orig" in a field or a NamespaceInfo, a
// search attribute "AudienceId", or a History or a history blob, which always has a translatable event.
func (b *messageBuilder) recordTranslatable(m protoreflect.Message, field protoreflect.FieldDescriptor) {
	if !m.Has(field) {
		return
	}
	if _, ok := visitorRuleOf(field); !ok {
		return
	}
	value := m.Get(field)
	if field.Kind() == protoreflect.StringKind {
		if value.String() == "orig" {
			b.cov.translatable[field.FullName()] = true
		}
		return
	}
	if field.IsMap() && field.MapValue().Kind() == protoreflect.MessageKind &&
		field.MapValue().Message().FullName() == payloadName {
		// Search attributes as a map from name to payload.
		if value.Map().Has(protoreflect.ValueOfString("AudienceId").MapKey()) {
			b.cov.translatable[field.FullName()] = true
		}
		return
	}

	// The field holds messages: one, a list, or map values.
	var messages []protoreflect.Message
	switch {
	case field.IsList():
		for i := range value.List().Len() {
			messages = append(messages, value.List().Get(i).Message())
		}
	case field.IsMap():
		value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
			messages = append(messages, v.Message())
			return true
		})
	default:
		messages = append(messages, value.Message())
	}
	for _, msg := range messages {
		if isTranslatableMessage(msg) {
			b.cov.translatable[field.FullName()] = true
			return
		}
	}
}

// isTranslatableMessage reports whether msg, the value of a field with a visitor rule, holds a name
// that a test matcher translates.
func isTranslatableMessage(msg protoreflect.Message) bool {
	fields := msg.Descriptor().Fields()
	switch msg.Descriptor().FullName() {
	case dataBlobName:
		return isHistoryBlob(msg)
	case historyName:
		return msg.Get(fields.ByName("events")).List().Len() > 0
	case namespaceInfoName:
		return msg.Get(fields.ByName("name")).String() == "orig"
	case searchAttributesName:
		return msg.Get(fields.ByName("indexed_fields")).Map().Has(protoreflect.ValueOfString("AudienceId").MapKey())
	}
	return false
}

// isHistoryBlob reports whether blob is a proto3 DataBlob that holds a history with events.
func isHistoryBlob(blob protoreflect.Message) bool {
	fields := blob.Descriptor().Fields()
	if blob.Get(fields.ByName("encoding_type")).Enum() != enums.ENCODING_TYPE_PROTO3.Number() {
		return false
	}
	var events history.History
	return proto.Unmarshal(blob.Get(fields.ByName("data")).Bytes(), &events) == nil && len(events.Events) > 0
}

// fill sets the fields of m. Every field that is not in a oneof is set. Oneof j is unset or holds
// one of its fields, picked by variant + offset + j.
func (b *messageBuilder) fill(m protoreflect.Message, depth, offset int) {
	desc := m.Descriptor()
	c := b.variant + offset
	if desc.FullName() == dataBlobName && c%4 != 3 {
		m.Set(desc.Fields().ByName("encoding_type"), protoreflect.ValueOfEnum(enums.ENCODING_TYPE_PROTO3.Number()))
		m.Set(desc.Fields().ByName("data"), protoreflect.ValueOfBytes(b.historyBytes(depth, c)))
		return
	}
	fields := desc.Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		if oneof := field.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() {
			continue
		}
		b.setField(m, field, depth, c+i)
	}
	oneofs := desc.Oneofs()
	for j := range oneofs.Len() {
		oneof := oneofs.Get(j)
		if oneof.IsSynthetic() {
			continue
		}
		pick := (c + j) % (oneof.Fields().Len() + 1)
		if pick == oneof.Fields().Len() {
			b.cov.oneofChoices[oneofChoiceKey(oneof, nil)] = true
			continue
		}
		field := oneof.Fields().Get(pick)
		b.cov.oneofChoices[oneofChoiceKey(oneof, field)] = true
		b.setField(m, field, depth, c+field.Index())
	}
	// Like a history blob, a History message always gets an event with a name to translate.
	if desc.FullName() == historyName && depth < testMessageMaxDepth {
		events := m.Mutable(desc.Fields().ByName("events")).List()
		events.Append(protoreflect.ValueOfMessage(translatableEvent(c).ProtoReflect()))
	}
}

// translatableEvent returns an event from makeTranslatableEvents, picked by c, with a namespace or
// a search attribute that a test matcher translates.
func translatableEvent(c int) *history.HistoryEvent {
	// The last event from makeTranslatableEvents has no name to translate.
	events := makeTranslatableEvents("orig", "AudienceId", 16)[:4]
	return events[c%len(events)]
}

// setField sets field of m from c: the variant, plus the offset of m, plus the field index.
// A list gets c modulo 3 elements, each with its own offset. A map gets one or two entries, so that
// the key of its first entry is c and, like a string field, takes each of testStrings.
func (b *messageBuilder) setField(m protoreflect.Message, field protoreflect.FieldDescriptor, depth, c int) {
	defer b.recordTranslatable(m, field)
	switch {
	case field.IsMap():
		if depth >= testMessageMaxDepth {
			return
		}
		entries := m.Mutable(field).Map()
		for e := range 1 + c%2 {
			key := b.scalar(field.MapKey(), depth, c+e).MapKey()
			if isMessageField(field.MapValue()) {
				value := entries.NewValue()
				b.fill(value.Message(), depth+1, c+e)
				entries.Set(key, value)
			} else {
				entries.Set(key, b.scalar(field.MapValue(), depth, c+e+1))
			}
		}
	case field.IsList():
		if isMessageField(field) && depth >= testMessageMaxDepth {
			return
		}
		list := m.Mutable(field).List()
		for e := range c % 3 {
			if isMessageField(field) {
				element := list.NewElement()
				b.fill(element.Message(), depth+1, c+e)
				list.Append(element)
			} else {
				list.Append(b.scalar(field, depth, c+e))
			}
		}
	case isMessageField(field):
		if depth < testMessageMaxDepth {
			b.fill(m.Mutable(field).Message(), depth+1, c)
		}
	default:
		value := b.scalar(field, depth, c)
		m.Set(field, value)
		if isPlainStringField(field) {
			values := b.cov.stringValues[field.FullName()]
			if values == nil {
				values = map[string]bool{}
				b.cov.stringValues[field.FullName()] = values
			}
			values[value.String()] = true
		}
	}
}

func isMessageField(field protoreflect.FieldDescriptor) bool {
	return field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind
}

// scalar returns a value of the kind of field, picked by c.
func (b *messageBuilder) scalar(field protoreflect.FieldDescriptor, depth, c int) protoreflect.Value {
	switch field.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(c%2 == 0)
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		return protoreflect.ValueOfEnum(values.Get(c % values.Len()).Number())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(int32(c % 1000))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(int64(c))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(uint32(c % 1000))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(uint64(c))
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(float32(c%1000) / 8)
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(float64(c) / 8)
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(testStrings[c%len(testStrings)])
	case protoreflect.BytesKind:
		switch c % 3 {
		case 0:
			// Translatable names as raw bytes. The visitors must leave them unchanged.
			return protoreflect.ValueOfBytes([]byte("orig AudienceId"))
		case 1:
			// Bytes that are not a history.
			data := make([]byte, c%64)
			for i := range data {
				data[i] = byte(c*31 + i*7)
			}
			return protoreflect.ValueOfBytes(data)
		default:
			return protoreflect.ValueOfBytes(b.historyBytes(depth, c))
		}
	default:
		panic("unhandled field kind " + field.Kind().String())
	}
}

// historyBytes returns a serialized history of zero to two events that the builder fills and, at
// a place picked by c, one event from makeTranslatableEvents with a namespace or a search attribute
// that a test matcher translates. Without that event most blobs would have nothing to translate,
// and the walk and skip runs would only be compared on blobs that they both ignore.
func (b *messageBuilder) historyBytes(depth, c int) []byte {
	if depth >= testMessageMaxDepth {
		return nil
	}
	var events history.History
	for e := range c % 3 {
		var event history.HistoryEvent
		b.fill(event.ProtoReflect(), depth+1, c+e)
		events.Events = append(events.Events, &event)
	}
	events.Events = slices.Insert(events.Events, c%(len(events.Events)+1), translatableEvent(c))
	data, err := proto.Marshal(&events)
	if err != nil {
		panic(err)
	}
	return data
}
