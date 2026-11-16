package interceptor

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/proto"
)

// largePayloadSize is large enough that a visitor that walks every byte
// allocates far more than one that skips byte slices.
const largePayloadSize = 64 << 10

var historyVisitors = []struct {
	name  string
	visit visitor
}{
	{name: "namespace", visit: visitNamespace},
	{name: "search attribute", visit: visitSearchAttributes},
}

// makeStartedEventWithLargePayload returns a started event that has a
// namespace field, a search attribute and a large input payload.
func makeStartedEventWithLargePayload(namespace, searchAttribute string) *history.HistoryEvent {
	return &history.HistoryEvent{
		EventId:   1,
		EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
		Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
				ParentWorkflowNamespace: namespace,
				Input: &common.Payloads{Payloads: []*common.Payload{{
					Metadata: map[string][]byte{"encoding": []byte("binary/plain")},
					Data:     bytes.Repeat([]byte{0xab}, largePayloadSize),
				}}},
				SearchAttributes: &common.SearchAttributes{
					IndexedFields: map[string]*common.Payload{
						searchAttribute: {Data: []byte(`"value"`)},
					},
				},
			},
		},
	}
}

func makeRawHistoryBlobWithLargePayload(t *testing.T, namespace, searchAttribute string) *common.DataBlob {
	blob, err := serializer.SerializeEvents([]*history.HistoryEvent{
		makeStartedEventWithLargePayload(namespace, searchAttribute),
	})
	require.NoError(t, err)
	return blob
}

func TestIsByteSlice(t *testing.T) {
	testcases := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "byte slice", value: []byte("data"), want: true},
		{name: "empty byte slice", value: []byte{}, want: true},
		{name: "nil byte slice", value: []byte(nil), want: true},
		{name: "named byte slice", value: json.RawMessage(`{}`), want: true},
		{name: "byte array", value: [4]byte{}, want: false},
		{name: "int8 slice", value: []int8{1}, want: false},
		{name: "string", value: "data", want: false},
		{name: "slice of byte slices", value: [][]byte{{1}}, want: false},
		{name: "map of byte slices", value: map[string][]byte{"k": {1}}, want: false},
		{name: "data blob slice", value: []*common.DataBlob{{}}, want: false},
	}
	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isByteSlice(reflect.ValueOf(tc.value)))
		})
	}
}

func TestVisitorsTranslateRawHistoryWithLargePayload(t *testing.T) {
	mapping := createStringMatcher(map[string]string{"orig": "orig.cloud"})
	expected := map[string]*history.HistoryEvent{
		"namespace":        makeStartedEventWithLargePayload("orig.cloud", "orig"),
		"search attribute": makeStartedEventWithLargePayload("orig", "orig.cloud"),
	}

	for _, v := range historyVisitors {
		t.Run(v.name, func(t *testing.T) {
			resp := &adminservice.GetWorkflowExecutionRawHistoryV2Response{
				NextPageToken:  []byte("next-page-token"),
				HistoryBatches: []*common.DataBlob{makeRawHistoryBlobWithLargePayload(t, "orig", "orig")},
			}

			changed, err := v.visit(log.NewTestLogger(), resp, mapping)
			require.NoError(t, err)
			require.True(t, changed)

			events, err := serializer.DeserializeEvents(resp.HistoryBatches[0])
			require.NoError(t, err)
			require.Len(t, events, 1)
			require.True(t, proto.Equal(expected[v.name], events[0]), "translated event: %v", events[0])
			require.Equal(t, []byte("next-page-token"), resp.NextPageToken)
		})
	}
}

func TestVisitorsDoNotWalkByteSlices(t *testing.T) {
	mapping := createStringMatcher(map[string]string{"orig": "orig.cloud"})
	blob := makeRawHistoryBlobWithLargePayload(t, "orig", "orig")
	logger := log.NewTestLogger()

	for _, v := range historyVisitors {
		t.Run(v.name, func(t *testing.T) {
			allocs := testing.AllocsPerRun(3, func() {
				// Translation replaces the blob in the response, so give each run its own response.
				resp := &adminservice.GetWorkflowExecutionRawHistoryV2Response{
					HistoryBatches: []*common.DataBlob{blob},
				}
				if _, err := v.visit(logger, resp, mapping); err != nil {
					t.Fatal(err)
				}
			})
			// A visitor that walks each byte allocates at least once for each byte of the
			// payload and of the serialized blob.
			require.Less(t, allocs, float64(largePayloadSize/16))
		})
	}
}

func BenchmarkVisitNamespace(b *testing.B) {
	variants := []struct {
		testName    string
		inputNSName string
		mapping     map[string]string
	}{
		{
			testName:    "name changed",
			inputNSName: "orig",
			mapping:     map[string]string{"orig": "orig.cloud"},
		},
		{
			testName:    "name unchanged",
			inputNSName: "orig",
			mapping:     map[string]string{"other": "other.cloud"},
		},
	}
	cases := generateNamespaceObjCases()

	logger := log.NewTestLogger()
	for _, c := range cases {
		b.Run(c.objName, func(b *testing.B) {
			for _, variant := range variants {
				translator := createStringMatcher(variant.mapping)
				b.Run(variant.testName, func(b *testing.B) {
					for i := 0; i < b.N; i++ {
						b.StopTimer()
						input := c.makeType(variant.inputNSName)

						b.StartTimer()
						_, _ = visitNamespace(logger, input, translator)
					}
				})
			}
		})
	}
}
