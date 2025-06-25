package interceptor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/persistence/serialization"
)

type (
	Other struct{}
	SA1   struct {
		Other            *Other
		SearchAttributes *common.SearchAttributes
	}

	SA2 struct {
		SearchAttributes *common.SearchAttributes
	}
)

func TestTranslateSearchAttribute(t *testing.T) {
	testTranslateObj(t, visitSearchAttributes, generateSearchAttributeObjs(), require.EqualExportedValues)
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
			objName:     "nil contrived SA1",
			containsObj: false,
			makeType: func(name string) any {
				return &SA1{}
			},
		},
		{
			objName:     "nil contrived SA2",
			containsObj: false,
			makeType: func(name string) any {
				return &SA2{}
			},
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
	blob, err := s.SerializeEvents(evts, enums.ENCODING_TYPE_PROTO3)
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
