package interceptor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/sdk/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/persistence/serialization"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestTranslateSearchAttribute(t *testing.T) {
	namespaceId := "ns-1234"
	testTranslateObj(t, visitSearchAttributes, generateSearchAttributeObjs(namespaceId), require.EqualExportedValues,
		func(mapping map[string]string) saMatcher {
			return func(nsId string) stringMatcher {
				if nsId != namespaceId {
					return nil
				}
				return createStringMatcher(mapping)
			}
		},
	)
}

func generateSearchAttributeObjs(nsId string) []objCase {
	return []objCase{
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
											NamespaceId:  nsId,
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
																NamespaceId:      nsId,
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
			EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
			Version:   1,
			TaskId:    100,
			Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
				WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
					WorkflowType: &common.WorkflowType{
						Name: "some-wf-type",
					},
					SearchAttributes: &common.SearchAttributes{
						IndexedFields: makeTestIndexedFieldMap(name),
					},
				},
			},
			EventTime:       &timestamppb.Timestamp{},
			WorkerMayIgnore: false,
			UserMetadata:    &sdk.UserMetadata{},
			Links:           []*common.Link{},
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
