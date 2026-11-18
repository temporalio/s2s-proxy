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
	testTranslateObj(t, visitSearchAttributesWithOneMapping, generateSearchAttributeObjs(), require.EqualExportedValues)
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

// testResolver returns the mapping in nsMappings for each namespace id, and no mapping for an
// id that is not in nsMappings.
func testResolver(nsMappings map[string]map[string]string) saMatcherResolver {
	matchers := createStringMatchers(nsMappings)
	return func(nsID string) (stringMatcher, bool) {
		match, ok := matchers[nsID]
		return match, ok
	}
}

// visitWithTestMappings visits obj with the mapping in testSAMappings for each namespace, and
// reports whether anything matched.
func visitWithTestMappings(t *testing.T, obj any, fallbackNamespaceID string) bool {
	t.Helper()
	changed, err := visitSearchAttributes(log.NewTestLogger(), obj, testResolver(testSAMappings()), fallbackNamespaceID)
	require.NoError(t, err)
	return changed
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
	// The visit order of the four tasks is unspecified, so the result must not depend on it.
	frame := makeMultiNamespaceFrame(testSAName)

	require.True(t, visitWithTestMappings(t, frame, ""))

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

func TestTranslateSearchAttributeInBlobUsesEnclosingNamespace(t *testing.T) {
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

	require.True(t, visitWithTestMappings(t, frame, ""))
	require.Equal(t, []string{keywordA}, blobSAKeys(t, firstTaskEvents(frame)))
}

// makeStartChildEvent returns a StartChildWorkflowExecutionInitiated event for a child in the
// namespace with the given id. Its search attributes have the key testSAName.
func makeStartChildEvent(childNamespaceID string) *history.HistoryEvent {
	return &history.HistoryEvent{
		EventId:   2,
		EventType: enums.EVENT_TYPE_START_CHILD_WORKFLOW_EXECUTION_INITIATED,
		Attributes: &history.HistoryEvent_StartChildWorkflowExecutionInitiatedEventAttributes{
			StartChildWorkflowExecutionInitiatedEventAttributes: &history.StartChildWorkflowExecutionInitiatedEventAttributes{
				Namespace:   "child-ns",
				NamespaceId: childNamespaceID,
				SearchAttributes: &common.SearchAttributes{
					IndexedFields: makeTestIndexedFieldMap(testSAName),
				},
			},
		},
	}
}

func startChildSAKeys(evt *history.HistoryEvent) []string {
	return mapKeys(evt.GetStartChildWorkflowExecutionInitiatedEventAttributes().GetSearchAttributes().GetIndexedFields())
}

func TestTranslateSearchAttributeStartChildUsesChildNamespace(t *testing.T) {
	// The ns-a parent's history has its own started event and a StartChildWorkflowExecutionInitiated
	// event. The server unaliases the child's search attributes with the child namespace and starts
	// the child with them, so each event in the same blob needs a different mapping.
	parentStarted := func() *history.HistoryEvent {
		return &history.HistoryEvent{
			EventId:   1,
			EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
					SearchAttributes: &common.SearchAttributes{
						IndexedFields: makeTestIndexedFieldMap(testSAName),
					},
				},
			},
		}
	}

	tests := []struct {
		name             string
		childNamespaceID string
		wantChild        string
	}{
		{name: "child in another namespace", childNamespaceID: testNsB, wantChild: keywordB},
		{name: "child in the same namespace", childNamespaceID: testNsA, wantChild: keywordA},
		// Old servers wrote no NamespaceId, so the event falls back to the enclosing owner. For a
		// cross-namespace child, the parent's mapping is wrong. This is a known limitation.
		{name: "child namespace id not set", childNamespaceID: "", wantChild: keywordA},
		// The parent's mapping must not apply to a child namespace that has no mapping.
		{name: "child namespace has no mapping", childNamespaceID: testNsC, wantChild: testSAName},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			frame := makeHistoryTaskFrame(testNsA, parentStarted(), makeStartChildEvent(tc.childNamespaceID))

			require.True(t, visitWithTestMappings(t, frame, ""))

			events, err := serialization.NewSerializer().DeserializeEvents(firstTaskEvents(frame))
			require.NoError(t, err)
			require.Len(t, events, 2)
			require.Equal(t, []string{keywordA},
				mapKeys(events[0].GetWorkflowExecutionStartedEventAttributes().GetSearchAttributes().GetIndexedFields()),
				"parent started event")
			require.Equal(t, []string{tc.wantChild}, startChildSAKeys(events[1]), "start child event")
		})
	}
}

func TestTranslateSearchAttributeUsesNearestOwner(t *testing.T) {
	// Outside a blob, the StartChild event and the ns-a task are both owners on one parent chain.
	// The nearest owner with a NamespaceId applies. An owner with an empty NamespaceId passes the
	// lookup to the next owner up. The server never buffers this event type, but the rule is the
	// same for any nested owners.
	newTask := func(childNamespaceID string) *replicationspb.SyncVersionedTransitionTaskAttributes {
		return &replicationspb.SyncVersionedTransitionTaskAttributes{
			NamespaceId: testNsA,
			VersionedTransitionArtifact: &replicationspb.VersionedTransitionArtifact{
				StateAttributes: &replicationspb.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes{
					SyncWorkflowStateSnapshotAttributes: &replicationspb.SyncWorkflowStateSnapshotAttributes{
						State: &persistence.WorkflowMutableState{
							BufferedEvents: []*history.HistoryEvent{makeStartChildEvent(childNamespaceID)},
						},
					},
				},
			},
		}
	}
	bufferedEvent := func(task *replicationspb.SyncVersionedTransitionTaskAttributes) *history.HistoryEvent {
		return task.GetVersionedTransitionArtifact().GetSyncWorkflowStateSnapshotAttributes().GetState().GetBufferedEvents()[0]
	}

	t.Run("child namespace id set", func(t *testing.T) {
		task := newTask(testNsB)
		require.True(t, visitWithTestMappings(t, task, ""))
		require.Equal(t, []string{keywordB}, startChildSAKeys(bufferedEvent(task)))
	})

	t.Run("child namespace id not set", func(t *testing.T) {
		task := newTask("")
		require.True(t, visitWithTestMappings(t, task, ""))
		require.Equal(t, []string{keywordA}, startChildSAKeys(bufferedEvent(task)))
	})
}

// nonOwnerWithNamespaceID has a NamespaceId field but is not in namespaceIDOwners.
type nonOwnerWithNamespaceID struct {
	NamespaceId      string
	SearchAttributes *common.SearchAttributes
}

func TestTranslateSearchAttributeOwnersAreAnAllowlist(t *testing.T) {
	// A NamespaceId field alone does not make a type an owner. This test fails if the allowlist
	// is replaced with a match on the field name.
	obj := &nonOwnerWithNamespaceID{
		NamespaceId:      testNsB,
		SearchAttributes: &common.SearchAttributes{IndexedFields: makeTestIndexedFieldMap(testSAName)},
	}
	require.True(t, visitWithTestMappings(t, obj, testNsA))
	require.Equal(t, []string{keywordA}, mapKeys(obj.SearchAttributes.GetIndexedFields()))
}

func TestTranslateSearchAttributeIgnoresParentNamespaceId(t *testing.T) {
	// WorkflowExecutionInfo carries both NamespaceId and ParentNamespaceId. Only the former
	// owns the search attributes.
	execInfo := &persistence.WorkflowExecutionInfo{
		NamespaceId:       testNsA,
		ParentNamespaceId: testNsB,
		SearchAttributes:  makeTestIndexedFieldMap(testSAName),
	}

	require.True(t, visitWithTestMappings(t, execInfo, ""))
	require.Equal(t, []string{keywordA}, mapKeys(execInfo.GetSearchAttributes()))
}

func TestTranslateSearchAttributeOneMappingAppliesToEveryNamespace(t *testing.T) {
	// The translator applies its one mapping to every search attribute, whatever namespace owns
	// it and also where no namespace resolves. The mapping can have an empty namespace id.
	for _, mappingNamespaceID := range []string{"", testNsA} {
		t.Run("mapping namespace id "+mappingNamespaceID, func(t *testing.T) {
			tr := newTestSATranslator(t, map[string]map[string]string{mappingNamespaceID: {testSAName: keywordA}})

			frame := makeMultiNamespaceFrame(testSAName)
			changed, err := tr.TranslateResponse(frame)
			require.NoError(t, err)
			require.True(t, changed)
			tasks := frame.GetMessages().GetReplicationTasks()
			require.Len(t, tasks, 4)
			require.Equal(t, []string{keywordA, keywordA}, blobSAKeys(t, tasks[0].GetHistoryTaskAttributes().GetEvents()))
			require.Equal(t, []string{keywordA, keywordA}, blobSAKeys(t, tasks[0].GetHistoryTaskAttributes().GetNewRunEvents()))
			require.Equal(t, []string{keywordA, keywordA}, blobSAKeys(t, tasks[1].GetHistoryTaskAttributes().GetEvents()))
			execInfo := tasks[2].GetSyncWorkflowStateTaskAttributes().GetWorkflowState().GetExecutionInfo()
			require.Equal(t, []string{keywordA}, mapKeys(execInfo.GetSearchAttributes()))
			require.Equal(t, []string{keywordA, keywordA}, blobSAKeys(t, tasks[3].GetHistoryTaskAttributes().GetEvents()))

			// No namespace owner encloses these history batches.
			resp := &adminservice.GetWorkflowExecutionRawHistoryV2Response{
				HistoryBatches: []*common.DataBlob{makeHistoryEventsBlobWithSearchAttribute(testSAName)},
			}
			changed, err = tr.TranslateResponse(resp)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, []string{keywordA, keywordA}, blobSAKeys(t, resp.GetHistoryBatches()[0]))

			req := &adminservice.ImportWorkflowExecutionRequest{
				HistoryBatches: []*common.DataBlob{makeHistoryEventsBlobWithSearchAttribute(testSAName)},
			}
			changed, err = tr.TranslateRequest(req)
			require.NoError(t, err)
			require.True(t, changed)
			require.Equal(t, []string{keywordA, keywordA}, blobSAKeys(t, req.GetHistoryBatches()[0]))
		})
	}
}
