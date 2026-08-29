package interceptor

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/persistence/serialization"
)

type (
	Other           struct{}
	SAWithTwoFields struct {
		Other            *Other
		SearchAttributes *common.SearchAttributes
	}

	SAWithTwoFieldsSwapped struct {
		SearchAttributes *common.SearchAttributes
		Other            *Other
	}

	SAWithOneField struct {
		SearchAttributes *common.SearchAttributes
	}

	SAWithOneMap struct {
		SearchAttributes map[string]*common.Payload
	}

	SAWithOneMapAndOneField struct {
		Other            *Other
		SearchAttributes map[string]*common.Payload
	}

	SAWithOneMapAndOneFieldSwapped struct {
		SearchAttributes map[string]*common.Payload
		Other            *Other
	}
)

func TestTranslateSearchAttribute(t *testing.T) {
	// These cases exercise a single namespace, so one matcher applies everywhere.
	adapter := func(l log.Logger, obj any, m stringMatcher) (bool, error) {
		return visitSearchAttributes(l, obj, constMatcherResolver(m), "")
	}
	testTranslateObj(t, adapter, generateSearchAttributeObjs(), require.EqualExportedValues)
}

func generateSearchAttributeObjs() []objCase {
	return []objCase{
		{
			objName:     "nil",
			containsObj: false,
			makeType: func(name string) any {
				return nil
			},
		},
		{
			objName:     "nil SearchAttributes",
			containsObj: false,
			makeType: func(name string) any {
				return &persistence.WorkflowExecutionInfo{
					NamespaceId:      name,
					SearchAttributes: map[string]*common.Payload(nil),
				}
			},
		},
		{
			objName:  "nil two fields",
			makeType: func(name string) any { return &SAWithTwoFields{} },
		},
		{
			objName:  "nil two fields different order",
			makeType: func(name string) any { return &SAWithTwoFieldsSwapped{} },
		},
		{
			objName:  "nil one field",
			makeType: func(name string) any { return &SAWithOneField{} },
		},
		{
			objName:  "nil map",
			makeType: func(name string) any { return &SAWithOneMap{} },
		},
		{
			objName:  "nil map and field",
			makeType: func(name string) any { return &SAWithOneMapAndOneField{} },
		},
		{
			objName:  "nil map and field different order",
			makeType: func(name string) any { return &SAWithOneMapAndOneFieldSwapped{} },
		},
		{
			objName:     "nil value in SearchAttributes",
			containsObj: true,
			makeType: func(name string) any {
				return &persistence.WorkflowExecutionInfo{
					SearchAttributes: map[string]*common.Payload{
						name: nil,
					},
				}
			},
		},
		{
			objName:     "HistoryTaskAttributes",
			containsObj: true,
			makeType: func(name string) any {
				return &adminservice.StreamWorkflowReplicationMessagesResponse{
					Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
						Messages: &replicationspb.WorkflowReplicationMessages{
							ReplicationTasks: []*replicationspb.ReplicationTask{
								{
									Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
										HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
											NamespaceId:  "some-ns-id",
											WorkflowId:   "some-wf-id",
											RunId:        "some-run-id",
											Events:       makeHistoryEventsBlobWithSearchAttribute(name),
											NewRunEvents: makeHistoryEventsBlobWithSearchAttribute(name),
										},
									},
								},
							},
						},
					},
				}
			},
		},
		{
			objName:     "SyncVersionedTransitionTaskAttributes",
			containsObj: true,
			makeType: func(name string) any {
				return &adminservice.StreamWorkflowReplicationMessagesResponse{
					Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
						Messages: &replicationspb.WorkflowReplicationMessages{
							ReplicationTasks: []*replicationspb.ReplicationTask{
								{
									Attributes: &replicationspb.ReplicationTask_SyncVersionedTransitionTaskAttributes{
										SyncVersionedTransitionTaskAttributes: &replicationspb.SyncVersionedTransitionTaskAttributes{
											VersionedTransitionArtifact: &replicationspb.VersionedTransitionArtifact{
												StateAttributes: &replicationspb.VersionedTransitionArtifact_SyncWorkflowStateMutationAttributes{
													SyncWorkflowStateMutationAttributes: &replicationspb.SyncWorkflowStateMutationAttributes{
														StateMutation: &persistence.WorkflowMutableStateMutation{
															ExecutionInfo: &persistence.WorkflowExecutionInfo{
																NamespaceId:      "some-ns",
																WorkflowId:       "some-wf",
																SearchAttributes: makeTestIndexedFieldMap(name),
																Memo: map[string]*common.Payload{
																	"orig": {
																		Data: []byte("the Memo field is the exacty same type as SearchAttributes but don't change it"),
																	},
																},
															},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
			},
		},
	}

}

func makeHistoryEventsBlobWithSearchAttribute(name string) *common.DataBlob {
	evts := []*history.HistoryEvent{
		{
			EventId:   1,
			EventType: enums.EVENT_TYPE_CHILD_WORKFLOW_EXECUTION_STARTED,
			Version:   1,
			TaskId:    100,
			Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
					WorkflowType: &common.WorkflowType{Name: "some-wf-type-1"},
					SearchAttributes: &common.SearchAttributes{
						IndexedFields: makeTestIndexedFieldMap(name),
					},
				},
			},
		},
		{
			Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
					WorkflowType:     &common.WorkflowType{Name: "some-wf-type-2"},
					SearchAttributes: nil,
				},
			},
		},
		{
			Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
					WorkflowType: &common.WorkflowType{Name: "some-wf-type-2"},
					SearchAttributes: &common.SearchAttributes{
						IndexedFields: nil,
					},
				},
			},
		},
		{
			Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
					WorkflowType: &common.WorkflowType{Name: "some-wf-type-3"},
					SearchAttributes: &common.SearchAttributes{
						IndexedFields: map[string]*common.Payload{
							name: nil,
						},
					},
				},
			},
		},
	}

	s := serialization.NewSerializer()
	blob, err := s.SerializeEvents(evts)
	if err != nil {
		panic(err)
	}
	return blob
}

func makeTestIndexedFieldMap(name string) map[string]*common.Payload {
	return map[string]*common.Payload{
		name: {
			Metadata: map[string][]byte{"preserve": []byte("this")},
			Data:     []byte("and this"),
		},
	}
}

const (
	testNsA = "ns-a"
	testNsB = "ns-b"
	testNsC = "ns-c"

	testSAName = "TestSA"
	keywordA   = "Keyword01"
	keywordB   = "Keyword02"
)

// testSAMappings maps the same search attribute to a different indexed field per namespace,
// so a translation applied with the wrong namespace's mapping is visible in assertions.
func testSAMappings() map[string]map[string]string {
	return map[string]map[string]string{
		testNsA: {testSAName: keywordA},
		testNsB: {testSAName: keywordB},
	}
}

func newTestSATranslator(t *testing.T, nsMappings map[string]map[string]string) Translator {
	t.Helper()
	return NewSearchAttributeTranslator(log.NewTestLogger(), nsMappings, nsMappings)
}

// makeMultiNamespaceFrame builds one replication frame carrying four tasks spanning three
// namespaces, which is what the wire actually looks like: resolution has to happen per subtree.
//
// Every blob and IndexedFields map is built fresh. visit.Values keeps a set of pointers it has
// already seen and skips repeats, so a hoisted, shared blob would leave the second subtree
// untranslated and could still read as a pass.
func makeMultiNamespaceFrame(saName string) *adminservice.StreamWorkflowReplicationMessagesResponse {
	return &adminservice.StreamWorkflowReplicationMessagesResponse{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
			Messages: &replicationspb.WorkflowReplicationMessages{
				ReplicationTasks: []*replicationspb.ReplicationTask{
					{
						Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
							HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
								NamespaceId:  testNsA,
								WorkflowId:   "wf-a",
								Events:       makeHistoryEventsBlobWithSearchAttribute(saName),
								NewRunEvents: makeHistoryEventsBlobWithSearchAttribute(saName),
							},
						},
					},
					{
						Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
							HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
								NamespaceId: testNsB,
								WorkflowId:  "wf-b",
								Events:      makeHistoryEventsBlobWithSearchAttribute(saName),
							},
						},
					},
					{
						// WorkflowExecutionInfo owns its namespace directly: no blob involved.
						Attributes: &replicationspb.ReplicationTask_SyncWorkflowStateTaskAttributes{
							SyncWorkflowStateTaskAttributes: &replicationspb.SyncWorkflowStateTaskAttributes{
								WorkflowState: &persistence.WorkflowMutableState{
									ExecutionInfo: &persistence.WorkflowExecutionInfo{
										NamespaceId:      testNsA,
										WorkflowId:       "wf-a-state",
										SearchAttributes: makeTestIndexedFieldMap(saName),
										// Memo is the same type as SearchAttributes. It must not be rewritten.
										Memo: makeTestIndexedFieldMap(saName),
									},
								},
							},
						},
					},
					{
						Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
							HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
								NamespaceId: testNsC,
								WorkflowId:  "wf-c",
								Events:      makeHistoryEventsBlobWithSearchAttribute(saName),
							},
						},
					},
				},
			},
		},
	}
}

// makeHistoryTaskFrame wraps the given events in a blob owned by nsID.
func makeHistoryTaskFrame(nsID string, events ...*history.HistoryEvent) *adminservice.StreamWorkflowReplicationMessagesResponse {
	blob, err := serialization.NewSerializer().SerializeEvents(events)
	if err != nil {
		panic(err)
	}
	return &adminservice.StreamWorkflowReplicationMessagesResponse{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
			Messages: &replicationspb.WorkflowReplicationMessages{
				ReplicationTasks: []*replicationspb.ReplicationTask{
					{
						Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
							HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
								NamespaceId: nsID,
								Events:      blob,
							},
						},
					},
				},
			},
		},
	}
}

func firstTaskEvents(resp *adminservice.StreamWorkflowReplicationMessagesResponse) *common.DataBlob {
	return resp.GetMessages().GetReplicationTasks()[0].GetHistoryTaskAttributes().GetEvents()
}

// blobSAKeys returns every search attribute key in the blob, sorted. Duplicates are kept so
// that a partial translation (one event rewritten, another missed) fails the assertion.
func blobSAKeys(t *testing.T, blob *common.DataBlob) []string {
	t.Helper()
	events, err := serialization.NewSerializer().DeserializeEvents(blob)
	require.NoError(t, err)

	var keys []string
	for _, evt := range events {
		keys = append(keys, mapKeys(evt.GetWorkflowExecutionStartedEventAttributes().GetSearchAttributes().GetIndexedFields())...)
		keys = append(keys, mapKeys(evt.GetStartChildWorkflowExecutionInitiatedEventAttributes().GetSearchAttributes().GetIndexedFields())...)
	}
	sort.Strings(keys)
	return keys
}

func mapKeys(fields map[string]*common.Payload) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func TestTranslateSearchAttributePerNamespace(t *testing.T) {
	tr := newTestSATranslator(t, testSAMappings())

	// A single pass can pass by luck: visit.ValuesUnsafe pops the front of its worklist but
	// swaps in the last element, so the order in which the four tasks are reached is
	// unspecified. Rebuild the frame every iteration so no subtree is ever seen pre-translated.
	for i := 0; i < 25; i++ {
		frame := makeMultiNamespaceFrame(testSAName)

		changed, err := tr.TranslateResponse(nil, frame)
		require.NoError(t, err)
		require.True(t, changed)

		tasks := frame.GetMessages().GetReplicationTasks()
		require.Len(t, tasks, 4)

		nsATask := tasks[0].GetHistoryTaskAttributes()
		require.Equal(t, []string{keywordA, keywordA}, blobSAKeys(t, nsATask.GetEvents()))
		require.Equal(t, []string{keywordA, keywordA}, blobSAKeys(t, nsATask.GetNewRunEvents()))

		require.Equal(t, []string{keywordB, keywordB}, blobSAKeys(t, tasks[1].GetHistoryTaskAttributes().GetEvents()),
			"ns-b must get its own mapping, not ns-a's")

		execInfo := tasks[2].GetSyncWorkflowStateTaskAttributes().GetWorkflowState().GetExecutionInfo()
		require.Equal(t, []string{keywordA}, mapKeys(execInfo.GetSearchAttributes()))
		require.Equal(t, []string{testSAName}, mapKeys(execInfo.GetMemo()), "Memo must not be rewritten")

		require.Equal(t, []string{testSAName, testSAName}, blobSAKeys(t, tasks[3].GetHistoryTaskAttributes().GetEvents()),
			"ns-c has no mapping and must be left untouched")
	}
}

func TestTranslateSearchAttributeInBlobUsesEnclosingNamespace(t *testing.T) {
	tr := newTestSATranslator(t, testSAMappings())

	// The event names a different namespace as its parent workflow's. The enclosing
	// HistoryTaskAttributes is what owns the namespace, so ns-a's mapping must win.
	frame := makeHistoryTaskFrame(testNsA, &history.HistoryEvent{
		EventId:   1,
		EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
		Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
				ParentWorkflowNamespaceId: testNsB,
				SearchAttributes: &common.SearchAttributes{
					IndexedFields: makeTestIndexedFieldMap(testSAName),
				},
			},
		},
	})

	changed, err := tr.TranslateResponse(nil, frame)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{keywordA}, blobSAKeys(t, firstTaskEvents(frame)))
}

func TestTranslateSearchAttributeIgnoresChildNamespaceId(t *testing.T) {
	tr := newTestSATranslator(t, testSAMappings())

	// StartChildWorkflowExecutionInitiatedEventAttributes has both a NamespaceId (the child's)
	// and SearchAttributes (the parent's). Resolving by nearest struct with a NamespaceId field
	// would translate this event with ns-b's mapping. This test fails if the owner allowlist is
	// ever replaced with a field-name match.
	frame := makeHistoryTaskFrame(testNsA, &history.HistoryEvent{
		EventId:   1,
		EventType: enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED,
		Attributes: &history.HistoryEvent_StartChildWorkflowExecutionInitiatedEventAttributes{
			StartChildWorkflowExecutionInitiatedEventAttributes: &history.StartChildWorkflowExecutionInitiatedEventAttributes{
				Namespace:   "child-ns",
				NamespaceId: testNsB,
				SearchAttributes: &common.SearchAttributes{
					IndexedFields: makeTestIndexedFieldMap(testSAName),
				},
			},
		},
	})

	changed, err := tr.TranslateResponse(nil, frame)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{keywordA}, blobSAKeys(t, firstTaskEvents(frame)))
}

func TestTranslateSearchAttributeIgnoresParentNamespaceId(t *testing.T) {
	tr := newTestSATranslator(t, testSAMappings())

	// WorkflowExecutionInfo carries both NamespaceId and ParentNamespaceId. Only the former
	// owns the search attributes.
	execInfo := &persistence.WorkflowExecutionInfo{
		NamespaceId:       testNsA,
		ParentNamespaceId: testNsB,
		SearchAttributes:  makeTestIndexedFieldMap(testSAName),
	}

	changed, err := tr.TranslateResponse(nil, execInfo)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, []string{keywordA}, mapKeys(execInfo.GetSearchAttributes()))
}

func TestTranslateSearchAttributeRawHistoryUsesPairedRequest(t *testing.T) {
	tr := newTestSATranslator(t, testSAMappings())

	// These responses carry history blobs but no namespace field of their own. Without the
	// paired request there is nothing to resolve, and the response must be left alone rather
	// than erroring or being translated with an arbitrary namespace's mapping.
	newV2Resp := func() *adminservice.GetWorkflowExecutionRawHistoryV2Response {
		return &adminservice.GetWorkflowExecutionRawHistoryV2Response{
			HistoryBatches: []*common.DataBlob{makeHistoryEventsBlobWithSearchAttribute(testSAName)},
		}
	}
	newResp := func() *adminservice.GetWorkflowExecutionRawHistoryResponse {
		return &adminservice.GetWorkflowExecutionRawHistoryResponse{
			HistoryBatches: []*common.DataBlob{makeHistoryEventsBlobWithSearchAttribute(testSAName)},
		}
	}

	t.Run("V2 unpaired", func(t *testing.T) {
		resp := newV2Resp()
		changed, err := tr.TranslateResponse(nil, resp)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, []string{testSAName, testSAName}, blobSAKeys(t, resp.HistoryBatches[0]))
	})

	t.Run("V2 paired", func(t *testing.T) {
		resp := newV2Resp()
		req := &adminservice.GetWorkflowExecutionRawHistoryV2Request{NamespaceId: testNsB}
		changed, err := tr.TranslateResponse(req, resp)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []string{keywordB, keywordB}, blobSAKeys(t, resp.HistoryBatches[0]))
	})

	t.Run("unpaired", func(t *testing.T) {
		resp := newResp()
		changed, err := tr.TranslateResponse(nil, resp)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, []string{testSAName, testSAName}, blobSAKeys(t, resp.HistoryBatches[0]))
	})

	t.Run("paired", func(t *testing.T) {
		resp := newResp()
		req := &adminservice.GetWorkflowExecutionRawHistoryRequest{NamespaceId: testNsB}
		changed, err := tr.TranslateResponse(req, resp)
		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, []string{keywordB, keywordB}, blobSAKeys(t, resp.HistoryBatches[0]))
	})
}

func TestTranslateSearchAttributeEmptyNamespaceIdMatchesNothing(t *testing.T) {
	// There is no mapping that applies to every namespace: an entry keyed by an empty namespace
	// id matches only an unresolved namespace, which no well formed replication task produces.
	// config.SATranslationConfig.Validate rejects such a config before it gets this far; this is
	// the second line of defence, and it fails if a wildcard fallback is ever reintroduced.
	tr := newTestSATranslator(t, map[string]map[string]string{"": {testSAName: keywordA}})

	frame := makeMultiNamespaceFrame(testSAName)
	changed, err := tr.TranslateResponse(nil, frame)
	require.NoError(t, err)
	require.False(t, changed)

	tasks := frame.GetMessages().GetReplicationTasks()
	require.Len(t, tasks, 4)
	for i, task := range tasks {
		if hta := task.GetHistoryTaskAttributes(); hta != nil {
			require.Equal(t, []string{testSAName, testSAName}, blobSAKeys(t, hta.GetEvents()),
				"task %d must be untouched", i)
			continue
		}
		execInfo := task.GetSyncWorkflowStateTaskAttributes().GetWorkflowState().GetExecutionInfo()
		require.Equal(t, []string{testSAName}, mapKeys(execInfo.GetSearchAttributes()),
			"task %d must be untouched", i)
	}
}
func TestTranslateSearchAttributeUnsupportedFieldTypes(t *testing.T) {
	// Add- and RemoveSearchAttributesRequest name their fields SearchAttributes too, but hold a
	// map[string]enums.IndexedValueType and a []string, so neither must be treated as an error
	// that aborts translation of the whole message.
	//
	// Neither request type is enclosed by a namespace owner, so the namespace resolves to "",
	// no matcher resolves, and the field is skipped before the type switch runs. That makes
	// visitSearchAttributes' unsupported-type branch defensive rather than load-bearing: it
	// only matters if a future message carries an odd SearchAttributes type *inside* a
	// namespace owner. Skipping is still the right outcome there, which is why it warns and
	// continues instead of returning visit.Stop.
	{
		t.Run("namespace keyed", func(t *testing.T) {
			tr := newTestSATranslator(t, testSAMappings())

			addReq := &adminservice.AddSearchAttributesRequest{
				SearchAttributes: map[string]enums.IndexedValueType{
					testSAName: enums.INDEXED_VALUE_TYPE_KEYWORD,
				},
			}
			changed, err := tr.TranslateRequest(addReq)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, map[string]enums.IndexedValueType{
				testSAName: enums.INDEXED_VALUE_TYPE_KEYWORD,
			}, addReq.SearchAttributes)

			removeReq := &adminservice.RemoveSearchAttributesRequest{
				SearchAttributes: []string{testSAName},
			}
			changed, err = tr.TranslateRequest(removeReq)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, []string{testSAName}, removeReq.SearchAttributes)
		})
	}
}
