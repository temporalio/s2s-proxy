package interceptor

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	workflowservicepb "go.temporal.io/api/workflowservice/v1"
	adminservicepb "go.temporal.io/server/api/adminservice/v1"
	_ "go.temporal.io/server/api/archiver/v1"
	clockpb "go.temporal.io/server/api/clock/v1"
	errordetailspb "go.temporal.io/server/api/errordetails/v1"
	_ "go.temporal.io/server/api/history/v1"
	_ "go.temporal.io/server/api/historyservice/v1"
	persistencepb "go.temporal.io/server/api/persistence/v1"
	replicationpb "go.temporal.io/server/api/replication/v1"
	_ "go.temporal.io/server/api/workflow/v1"
	"go.temporal.io/server/common/persistence/serialization"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/failoverversion"
)

func testFailoverTranslator(t *testing.T) (*FailoverVersionTranslator, *FailoverVersionTranslator) {
	t.Helper()
	mapping, err := failoverversion.NewMapping(config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: 105, Remote: 100},
		IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{
			{Local: 1, Remote: 1},
			{Local: 2, Remote: 2},
			{Local: 105, Remote: 100},
		}},
	})
	require.NoError(t, err)
	return NewFailoverVersionTranslator(mapping.LocalToRemote()), NewFailoverVersionTranslator(mapping.RemoteToLocal())
}

func TestFailoverVersionTranslatorDiscoveryAndNamespace(t *testing.T) {
	t.Parallel()
	forward, inverse := testFailoverTranslator(t)
	discovery := &adminservicepb.DescribeClusterResponse{
		FailoverVersionIncrement: 105,
		InitialFailoverVersion:   1,
	}
	result, err := forward.TranslateMessage(discovery)
	require.NoError(t, err)
	require.True(t, result.Report.Changed)
	discovery = result.Message.(*adminservicepb.DescribeClusterResponse)
	require.Equal(t, int64(100), discovery.FailoverVersionIncrement)
	require.Equal(t, int64(1), discovery.InitialFailoverVersion)

	namespace := &workflowservicepb.DescribeNamespaceResponse{FailoverVersion: 107}
	result, err = forward.TranslateMessage(namespace)
	require.NoError(t, err)
	require.True(t, result.Report.Changed)
	namespace = result.Message.(*workflowservicepb.DescribeNamespaceResponse)
	require.Equal(t, int64(102), namespace.FailoverVersion)
	result, err = inverse.TranslateMessage(namespace)
	require.NoError(t, err)
	namespace = result.Message.(*workflowservicepb.DescribeNamespaceResponse)
	require.Equal(t, int64(107), namespace.FailoverVersion)
}

func TestFailoverVersionTranslatorMutableStateClearsChecksum(t *testing.T) {
	t.Parallel()
	forward, _ := testFailoverTranslator(t)
	state := &persistencepb.WorkflowMutableState{
		ActivityInfos: map[int64]*persistencepb.ActivityInfo{1: {Version: 106, StartVersion: 107}},
		Checksum:      &persistencepb.Checksum{Version: 1, Value: []byte("old-domain")},
	}
	result, err := forward.TranslateMessage(state)
	require.NoError(t, err)
	require.True(t, result.Report.Changed)
	require.Equal(t, uint32(2), result.Report.VersionFields)
	require.Equal(t, uint32(1), result.Report.ChecksumsCleared)
	state = result.Message.(*persistencepb.WorkflowMutableState)
	require.Equal(t, int64(101), state.ActivityInfos[1].Version)
	require.Equal(t, int64(102), state.ActivityInfos[1].StartVersion)
	require.Nil(t, state.Checksum)

	unchanged := &persistencepb.WorkflowMutableState{Checksum: &persistencepb.Checksum{Version: 1}}
	result, err = forward.TranslateMessage(unchanged)
	require.NoError(t, err)
	require.False(t, result.Report.Changed)
	unchanged = result.Message.(*persistencepb.WorkflowMutableState)
	require.NotNil(t, unchanged.Checksum)
}

func TestFailoverVersionTranslatorHistoryBlob(t *testing.T) {
	t.Parallel()
	forward, _ := testFailoverTranslator(t)
	serializer := serialization.NewSerializer()
	blob, err := serializer.SerializeEvents([]*historypb.HistoryEvent{{EventId: 1, Version: 106}})
	require.NoError(t, err)
	attributes := &replicationpb.HistoryTaskAttributes{Events: blob}
	result, err := forward.TranslateMessage(attributes)
	require.NoError(t, err)
	require.True(t, result.Report.Changed)
	require.Equal(t, uint32(1), result.Report.HistoryBlobs)
	attributes = result.Message.(*replicationpb.HistoryTaskAttributes)
	events, err := serializer.DeserializeEvents(attributes.Events)
	require.NoError(t, err)
	require.Equal(t, int64(101), events[0].Version)
	require.Equal(t, blob.EncodingType, attributes.Events.EncodingType)
}

func TestFailoverVersionTranslatorReleasesLargeTransactionalClones(t *testing.T) {
	forward, _ := testFailoverTranslator(t)
	serializer := serialization.NewSerializer()
	largePayload := make([]byte, 4<<20)
	blob, err := serializer.SerializeEvents([]*historypb.HistoryEvent{{
		EventId: 1,
		Version: 106,
		Attributes: &historypb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &historypb.WorkflowExecutionStartedEventAttributes{
				Input: &commonpb.Payloads{Payloads: []*commonpb.Payload{{Data: largePayload}}},
			},
		},
	}})
	require.NoError(t, err)
	input := &replicationpb.HistoryTaskAttributes{Events: blob}

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	for range 64 {
		result, translateErr := forward.TranslateMessage(input)
		require.NoError(t, translateErr)
		require.True(t, result.Report.Changed)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	require.Less(t, after.HeapAlloc, before.HeapAlloc+64<<20,
		"completed translations must not retain large request/response clones")
}

func TestFailoverVersionTranslatorRejectsOpaqueReplicationData(t *testing.T) {
	t.Parallel()
	forward, _ := testFailoverTranslator(t)
	task := &replicationpb.ReplicationTask{
		Data: &commonpb.DataBlob{EncodingType: enumspb.ENCODING_TYPE_PROTO3, Data: []byte{1}},
	}
	_, err := forward.TranslateMessage(task)
	var translationErr *failoverversion.TranslationError
	require.Error(t, err)
	require.True(t, errors.As(err, &translationErr))
	require.Equal(t, failoverversion.ErrorUnsupported, translationErr.Code)
}

func TestFailoverVersionTranslatorExplicitExclusions(t *testing.T) {
	t.Parallel()
	forward, _ := testFailoverTranslator(t)
	attributes := &replicationpb.NamespaceTaskAttributes{ConfigVersion: 106, FailoverVersion: 106}
	result, err := forward.TranslateMessage(attributes)
	require.NoError(t, err)
	require.True(t, result.Report.Changed)
	attributes = result.Message.(*replicationpb.NamespaceTaskAttributes)
	require.Equal(t, int64(106), attributes.ConfigVersion)
	require.Equal(t, int64(101), attributes.FailoverVersion)

	clock := &clockpb.HybridLogicalClock{Version: 106, ClusterId: 1}
	result, err = forward.TranslateMessage(clock)
	require.NoError(t, err)
	require.False(t, result.Report.Changed)
	clock = result.Message.(*clockpb.HybridLogicalClock)
	require.Equal(t, int32(106), clock.Version)
	require.Equal(t, int64(1), clock.ClusterId)
}

func TestFailoverVersionTranslatorStatusDetails(t *testing.T) {
	t.Parallel()
	forward, _ := testFailoverTranslator(t)
	st, err := status.New(codes.Aborted, "retry replication").WithDetails(&errordetailspb.RetryReplicationFailure{
		StartEventVersion: 106,
		EndEventVersion:   107,
	})
	require.NoError(t, err)

	translation := forward.TranslateError(st.Err())
	translated, report := translation.Error, translation.Report
	require.True(t, report.Changed)
	require.Equal(t, uint32(1), report.StatusDetails)
	got, ok := status.FromError(translated)
	require.True(t, ok)
	require.Equal(t, codes.Aborted, got.Code())
	require.Equal(t, "retry replication", got.Message())
	details := got.Details()
	require.Len(t, details, 1)
	retry, ok := details[0].(*errordetailspb.RetryReplicationFailure)
	require.True(t, ok)
	require.Equal(t, int64(101), retry.StartEventVersion)
	require.Equal(t, int64(102), retry.EndEventVersion)
}

func TestFailoverVersionTranslatorStatusDetailsAreAtomic(t *testing.T) {
	t.Parallel()
	forward, _ := testFailoverTranslator(t)
	original, err := status.New(codes.Aborted, "retry replication").WithDetails(
		&errordetailspb.RetryReplicationFailure{StartEventVersion: 106, EndEventVersion: 107},
		&errordetailspb.RetryReplicationFailure{StartEventVersion: 106, EndEventVersion: 108},
	)
	require.NoError(t, err)

	translation := forward.TranslateError(original.Err())
	translated, report := translation.Error, translation.Report
	require.Error(t, translated)
	require.False(t, report.Changed)
	var translationErr *failoverversion.TranslationError
	require.ErrorAs(t, translated, &translationErr)

	unchanged, ok := status.FromError(original.Err())
	require.True(t, ok)
	details := unchanged.Details()
	require.Len(t, details, 2)
	require.Equal(t, int64(106), details[0].(*errordetailspb.RetryReplicationFailure).GetStartEventVersion())
	require.Equal(t, int64(108), details[1].(*errordetailspb.RetryReplicationFailure).GetEndEventVersion())
}

func TestFailoverVersionTranslatorIsTransactional(t *testing.T) {
	t.Parallel()
	forward, _ := testFailoverTranslator(t)
	original := &errordetailspb.RetryReplicationFailure{
		StartEventVersion: 106, // valid lane 1; visited before end_event_version
		EndEventVersion:   108, // unmapped lane 3
	}
	want := proto.Clone(original)

	result, err := forward.TranslateMessage(original)
	require.Error(t, err)
	require.Nil(t, result.Message)
	require.True(t, proto.Equal(want, original), "failed translation must not mutate its input")

	original.EndEventVersion = 107
	want = proto.Clone(original)
	result, err = forward.TranslateMessage(original)
	require.NoError(t, err)
	require.True(t, result.Report.Changed)
	require.True(t, proto.Equal(want, original), "successful translation must still leave its input unchanged")
	require.False(t, proto.Equal(original, result.Message))
}

func TestFailoverVersionTranslatorStagesBlobLists(t *testing.T) {
	t.Parallel()
	forward, _ := testFailoverTranslator(t)
	serializer := serialization.NewSerializer()
	valid, err := serializer.SerializeEvents([]*historypb.HistoryEvent{{EventId: 1, Version: 106}})
	require.NoError(t, err)
	invalid, err := serializer.SerializeEvents([]*historypb.HistoryEvent{{EventId: 2, Version: 108}})
	require.NoError(t, err)
	original := &replicationpb.HistoryTaskAttributes{EventsBatches: []*commonpb.DataBlob{valid, invalid}}
	want := proto.Clone(original)

	_, err = forward.TranslateMessage(original)
	require.Error(t, err)
	require.True(t, proto.Equal(want, original), "a later blob failure must not replace an earlier blob")
}

func TestFailoverVersionFieldRegistryMatchesLinkedDescriptors(t *testing.T) {
	t.Parallel()
	for name, want := range ClassifiedFailoverVersionFields() {
		descriptor, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
		require.NoErrorf(t, err, "classified field %s is no longer present", name)
		field, ok := descriptor.(protoreflect.FieldDescriptor)
		require.Truef(t, ok, "%s is no longer a field", name)
		got, classified := ClassifyFailoverVersionField(field)
		require.Truef(t, classified, "%s is not classified", name)
		require.Equal(t, want, got, name)
		if got.Kind == FailoverVersionFieldExcluded {
			require.NotEmpty(t, got.Reason, name)
		}
	}

	protoregistry.GlobalFiles.RangeFiles(func(file protoreflect.FileDescriptor) bool {
		walkDescriptorMessages(file.Messages(), func(field protoreflect.FieldDescriptor) {
			if !strings.Contains(string(field.Name()), "failover_version") {
				return
			}
			_, classified := ClassifyFailoverVersionField(field)
			require.Truef(t, classified, "new failover-version candidate requires review: %s", field.FullName())
		})
		return true
	})
}

func TestFailoverVersionTranslatorResetAndParentHistoryFields(t *testing.T) {
	t.Parallel()
	forward, inverse := testFailoverTranslator(t)
	original := &historypb.HistoryEvent{
		EventId: 1,
		Version: 107,
		Attributes: &historypb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &historypb.WorkflowExecutionStartedEventAttributes{
				ParentInitiatedEventVersion: 106,
			},
		},
	}
	result, err := forward.TranslateMessage(original)
	require.NoError(t, err)
	translated := result.Message.(*historypb.HistoryEvent)
	require.Equal(t, int64(102), translated.GetVersion())
	require.Equal(t, int64(101), translated.GetWorkflowExecutionStartedEventAttributes().GetParentInitiatedEventVersion())
	require.Equal(t, uint32(2), result.Report.VersionFields)

	result, err = inverse.TranslateMessage(translated)
	require.NoError(t, err)
	require.True(t, proto.Equal(original, result.Message))

	reset := &historypb.HistoryEvent{
		Version: 106,
		Attributes: &historypb.HistoryEvent_WorkflowTaskFailedEventAttributes{
			WorkflowTaskFailedEventAttributes: &historypb.WorkflowTaskFailedEventAttributes{ForkEventVersion: 107},
		},
	}
	result, err = forward.TranslateMessage(reset)
	require.NoError(t, err)
	require.Equal(t, int64(102), result.Message.(*historypb.HistoryEvent).GetWorkflowTaskFailedEventAttributes().GetForkEventVersion())
}

type faultingHistoryEventCodec struct {
	events    []*historypb.HistoryEvent
	decodeErr error
	encodeErr error
}

func (c faultingHistoryEventCodec) Decode(*commonpb.DataBlob) ([]*historypb.HistoryEvent, error) {
	return c.events, c.decodeErr
}

func (c faultingHistoryEventCodec) Encode(enumspb.EncodingType, []*historypb.HistoryEvent) (*commonpb.DataBlob, error) {
	if c.encodeErr != nil {
		return nil, c.encodeErr
	}
	return &commonpb.DataBlob{EncodingType: enumspb.ENCODING_TYPE_PROTO3, Data: []byte{1}}, nil
}

func TestFailoverVersionTranslatorCodecFailuresAreTypedAndAtomic(t *testing.T) {
	t.Parallel()
	mapping, err := failoverversion.NewMapping(config.FailoverVersionTranslationConfig{
		FVI: config.IntMapping{Local: 105, Remote: 100},
		IFV: config.IFVTranslationConfig{Mappings: []config.IntMapping{{Local: 1, Remote: 1}, {Local: 2, Remote: 2}}},
	})
	require.NoError(t, err)
	original := &replicationpb.HistoryTaskAttributes{Events: &commonpb.DataBlob{EncodingType: enumspb.ENCODING_TYPE_PROTO3, Data: []byte{1}}}

	decode := NewFailoverVersionTranslator(mapping.LocalToRemote(), WithHistoryEventCodec(faultingHistoryEventCodec{decodeErr: errors.New("decode secret")}))
	_, err = decode.TranslateMessage(original)
	requireErrorCode(t, err, failoverversion.ErrorDecodeFailed)
	require.Equal(t, []byte{1}, original.GetEvents().GetData())

	encode := NewFailoverVersionTranslator(mapping.LocalToRemote(), WithHistoryEventCodec(faultingHistoryEventCodec{
		events: []*historypb.HistoryEvent{{Version: 106}}, encodeErr: errors.New("encode secret"),
	}))
	_, err = encode.TranslateMessage(original)
	requireErrorCode(t, err, failoverversion.ErrorEncodeFailed)
	require.Equal(t, []byte{1}, original.GetEvents().GetData())
}

func requireErrorCode(t *testing.T, err error, code failoverversion.ErrorCode) {
	t.Helper()
	var translationErr *failoverversion.TranslationError
	require.Error(t, err)
	require.ErrorAs(t, err, &translationErr)
	require.Equal(t, code, translationErr.Code)
	require.NotContains(t, err.Error(), "secret")
}

func walkDescriptorMessages(messages protoreflect.MessageDescriptors, visit func(protoreflect.FieldDescriptor)) {
	for index := 0; index < messages.Len(); index++ {
		message := messages.Get(index)
		for fieldIndex := 0; fieldIndex < message.Fields().Len(); fieldIndex++ {
			visit(message.Fields().Get(fieldIndex))
		}
		walkDescriptorMessages(message.Messages(), visit)
	}
}
