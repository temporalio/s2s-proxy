package interceptor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/command/v1"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/replication/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	"go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/persistence/serialization"
)

type (
	StructWithNamespaceField struct {
		Namespace string
	}
	StructWithWorkflowNamespaceField struct {
		WorkflowNamespace string
	}
	StructWithNestedNamespaceField struct {
		Other  string
		Nested StructWithNamespaceField
	}
	StructWithListOfNestedNamespaceField struct {
		Other  string
		Nested []StructWithNamespaceField
	}
	StructWithListOfNestedPtrs struct {
		Other  string
		Nested []*StructWithNamespaceField
	}
	StructWithCircularPointer struct {
		Link      *StructWithCircularPointer
		Namespace string
	}

	objCase struct {
		objName     string
		containsObj bool
		makeType    func(ns string) any
		expError    string
	}
)

func TestTranslateNamespaceName(t *testing.T) {
	testTranslateObj(t, visitNamespace, generateNamespaceObjCases(), require.Equal,
		func(mapping map[string]string) stringMatcher {
			return createStringMatcher(mapping)
		},
	)
}

func TestTranslateNamespaceReplicationMessages(t *testing.T) {
	testTranslateObj(t, visitNamespace, generateNamespaceReplicationMessages(), require.EqualExportedValues,
		func(mapping map[string]string) stringMatcher {
			return createStringMatcher(mapping)
		},
	)
}

func generateNamespaceObjCases() []objCase {
	return []objCase{
		{
			objName:     "Namespace field",
			containsObj: true,
			makeType: func(ns string) any {
				return &StructWithNamespaceField{Namespace: ns}
			},
		},
		{
			objName:     "WorkflowNamespace field",
			containsObj: true,
			makeType: func(ns string) any {
				return &StructWithWorkflowNamespaceField{WorkflowNamespace: ns}
			},
		},
		{
			objName:     "Nested Namespace field",
			containsObj: true,
			makeType: func(ns string) any {
				return &StructWithNestedNamespaceField{
					Other: "do not change",
					Nested: StructWithNamespaceField{
						Namespace: ns,
					},
				}
			},
		},
		{
			objName:     "list of structs",
			containsObj: true,
			makeType: func(ns string) any {
				return &StructWithListOfNestedNamespaceField{
					Other: "do not change",
					Nested: []StructWithNamespaceField{
						{
							Namespace: ns,
						},
					},
				}
			},
		},
		{
			objName:     "list of ptrs",
			containsObj: true,
			makeType: func(ns string) any {
				return &StructWithListOfNestedPtrs{
					Other: "do not change",
					Nested: []*StructWithNamespaceField{
						{
							Namespace: ns,
						},
					},
				}
			},
		},
		{
			objName:     "RespondWorkflowTaskCompletedRequest",
			containsObj: true,
			makeType: func(ns string) any {
				return &workflowservice.RespondWorkflowTaskCompletedRequest{
					TaskToken: []byte{},
					Commands: []*command.Command{
						{
							CommandType: enums.COMMAND_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION,
							Attributes: &command.Command_SignalExternalWorkflowExecutionCommandAttributes{
								SignalExternalWorkflowExecutionCommandAttributes: &command.SignalExternalWorkflowExecutionCommandAttributes{
									Namespace:  ns,
									SignalName: "do-not-change",
								},
							},
						},
						{
							CommandType: enums.COMMAND_TYPE_START_CHILD_WORKFLOW_EXECUTION,
							Attributes: &command.Command_StartChildWorkflowExecutionCommandAttributes{
								StartChildWorkflowExecutionCommandAttributes: &command.StartChildWorkflowExecutionCommandAttributes{
									Namespace:  ns,
									WorkflowId: "do-not-change",
								},
							},
						},
					},
					Identity:  "do-not-change",
					Namespace: ns,
				}
			},
		},
		{
			objName:     "PollWorkflowTaskQueueResponse",
			containsObj: true,
			makeType: func(ns string) any {
				return &workflowservice.PollWorkflowTaskQueueResponse{
					TaskToken:              []byte{},
					PreviousStartedEventId: 0,
					StartedEventId:         0,
					Attempt:                0,
					BacklogCountHint:       0,
					History: &history.History{
						Events: []*history.HistoryEvent{
							{
								EventId:         0,
								EventType:       enums.EVENT_TYPE_SIGNAL_EXTERNAL_WORKFLOW_EXECUTION_INITIATED,
								Version:         0,
								TaskId:          0,
								WorkerMayIgnore: false,
								Attributes: &history.HistoryEvent_SignalExternalWorkflowExecutionInitiatedEventAttributes{
									SignalExternalWorkflowExecutionInitiatedEventAttributes: &history.SignalExternalWorkflowExecutionInitiatedEventAttributes{
										Namespace: ns,
									},
								},
							},
						},
					},
					NextPageToken: []byte{},
				}
			},
		},
		{
			objName:     "GetWorkflowExecutionRawHistoryV2Response",
			containsObj: true,
			makeType: func(ns string) any {
				return &adminservice.GetWorkflowExecutionRawHistoryV2Response{
					NextPageToken: []byte("some-token"),
					HistoryBatches: []*common.DataBlob{
						makeHistoryEventsBlobWithNamespaceField(ns),
						makeHistoryEventsBlobWithNamespaceField(ns),
					},
					HistoryNodeIds: []int64{123},
				}
			},
		},
		{
			objName:     "DescribeNamespaceResponse",
			containsObj: true,
			makeType: func(ns string) any {
				return workflowservice.DescribeNamespaceResponse{
					NamespaceInfo: &namespace.NamespaceInfo{
						Name: ns,
					},
				}
			},
			expError: "",
		},
		{
			objName:     "UpdateNamespaceResponse",
			containsObj: true,
			makeType: func(ns string) any {
				return workflowservice.UpdateNamespaceResponse{
					NamespaceInfo: &namespace.NamespaceInfo{
						Name:        ns,
						State:       1,
						Description: "test",
					},
					Config: &namespace.NamespaceConfig{},
					ReplicationConfig: &replication.NamespaceReplicationConfig{
						ActiveClusterName: "active",
						Clusters: []*replication.ClusterReplicationConfig{
							{
								ClusterName: "some-cluster",
							},
						},
					},
				}
			},
			expError: "",
		},
		{
			objName:     "ListNamespacesResponse",
			containsObj: true,
			makeType: func(ns string) any {
				return &workflowservice.ListNamespacesResponse{
					Namespaces: []*workflowservice.DescribeNamespaceResponse{
						{
							NamespaceInfo: &namespace.NamespaceInfo{Name: ns},
						},
					},
					NextPageToken: []byte{},
				}
			},
			expError: "",
		},
		{
			objName:     "StreamWorkflowReplicationMessagesResponse",
			containsObj: true,
			makeType: func(ns string) any {
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
											Events:       makeHistoryEventsBlobWithNamespaceField(ns),
											NewRunEvents: makeHistoryEventsBlobWithNamespaceField(ns),
										},
									},
								},
								{
									Attributes: &replicationspb.ReplicationTask_HistoryTaskAttributes{
										HistoryTaskAttributes: &replicationspb.HistoryTaskAttributes{
											NamespaceId:  "some-ns-id",
											WorkflowId:   "some-wf-id-2",
											RunId:        "some-run-id-2",
											Events:       makeHistoryEventsBlobWithNamespaceField(ns),
											NewRunEvents: makeHistoryEventsBlobWithNamespaceField(ns),
										},
									},
								},
								{
									Attributes: &replicationspb.ReplicationTask_SyncWorkflowStateTaskAttributes{
										SyncWorkflowStateTaskAttributes: &replicationspb.SyncWorkflowStateTaskAttributes{
											WorkflowState: &persistence.WorkflowMutableState{
												ActivityInfos: map[int64]*persistence.ActivityInfo{},
												TimerInfos:    map[string]*persistence.TimerInfo{},
												ChildExecutionInfos: map[int64]*persistence.ChildExecutionInfo{
													123: {
														Namespace:        ns,
														WorkflowTypeName: "some-type-name",
														NamespaceId:      "some-ns-id",
													},
												},
												BufferedEvents: []*history.HistoryEvent{
													{
														EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
														Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
															WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{
																ParentWorkflowNamespace:   ns,
																ParentWorkflowNamespaceId: "some-ns-id",
															},
														},
													},
												},
											},
										},
									},
								},
								{
									Attributes: &replicationspb.ReplicationTask_BackfillHistoryTaskAttributes{
										BackfillHistoryTaskAttributes: &replicationspb.BackfillHistoryTaskAttributes{
											NamespaceId: "some-ns-id",
											WorkflowId:  "some-wf-id",
											RunId:       "some-run-id",
											EventBatches: []*common.DataBlob{
												makeHistoryEventsBlobWithNamespaceField(ns),
												makeHistoryEventsBlobWithNamespaceField(ns),
											},
											NewRunInfo: &replicationspb.NewRunInfo{
												RunId:      "some-new-run-id",
												EventBatch: makeHistoryEventsBlobWithNamespaceField(ns),
											},
										},
									},
								},
								{
									Attributes: &replicationspb.ReplicationTask_SyncVersionedTransitionTaskAttributes{
										SyncVersionedTransitionTaskAttributes: &replicationspb.SyncVersionedTransitionTaskAttributes{
											VersionedTransitionArtifact: &replicationspb.VersionedTransitionArtifact{
												StateAttributes: nil,
												EventBatches: []*common.DataBlob{
													makeHistoryEventsBlobWithNamespaceField(ns),
													makeHistoryEventsBlobWithNamespaceField(ns),
												},
												NewRunInfo: &replicationspb.NewRunInfo{
													RunId:      "some-run-id",
													EventBatch: makeHistoryEventsBlobWithNamespaceField(ns),
												},
											},
											NamespaceId: "some-ns-id",
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
			objName:     "circular pointer",
			containsObj: true,
			makeType: func(ns string) any {
				a := &StructWithCircularPointer{
					Namespace: ns,
				}
				b := &StructWithCircularPointer{
					Namespace: ns,
				}
				a.Link = b
				b.Link = a
				return a
			},
		},
	}
}

func generateNamespaceReplicationMessages() []objCase {
	makeFullType := func(ns string) any {
		return &adminservice.GetNamespaceReplicationMessagesResponse{
			Messages: &replicationspb.ReplicationMessages{
				ReplicationTasks: []*replicationspb.ReplicationTask{
					{
						TaskType:     enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK,
						SourceTaskId: 123,
						Attributes: &replicationspb.ReplicationTask_NamespaceTaskAttributes{
							NamespaceTaskAttributes: &replicationspb.NamespaceTaskAttributes{
								NamespaceOperation: 345,
								Id:                 "abc",
								Info: &namespace.NamespaceInfo{
									Name:        ns,
									State:       789,
									Description: "do not change",
									OwnerEmail:  "not this either",
									Data:        map[string]string{"some": "val"},
									Id:          "",
									Capabilities: &namespace.NamespaceInfo_Capabilities{
										EagerWorkflowStart: true,
										SyncUpdate:         true,
										AsyncUpdate:        true,
									},
									SupportsSchedules: true,
								},
								ConfigVersion:   2,
								FailoverVersion: 3,
							},
						},
					},
				},
			},
		}
	}

	return []objCase{
		{
			objName: "nil",
			makeType: func(ns string) any {
				return nil
			},
		},
		{
			objName: "nil messages",
			makeType: func(ns string) any {
				return &adminservice.GetNamespaceReplicationMessagesResponse{}
			},
		},
		{
			objName: "nil replication tasks",
			makeType: func(ns string) any {
				return &adminservice.GetNamespaceReplicationMessagesResponse{
					Messages: &replicationspb.ReplicationMessages{
						ReplicationTasks: nil,
					},
				}
			},
		},
		{
			objName: "nil replication tasks item",
			makeType: func(ns string) any {
				return &adminservice.GetNamespaceReplicationMessagesResponse{
					Messages: &replicationspb.ReplicationMessages{
						ReplicationTasks: []*replicationspb.ReplicationTask{
							nil,
						},
						LastRetrievedMessageId: 1234,
					},
				}
			},
		},
		{
			objName: "nil attributes",
			makeType: func(ns string) any {
				return &adminservice.GetNamespaceReplicationMessagesResponse{
					Messages: &replicationspb.ReplicationMessages{
						ReplicationTasks: []*replicationspb.ReplicationTask{
							{},
						},
						LastRetrievedMessageId: 1234,
					},
				}
			},
		},
		{
			objName: "nil namespace task attributes",
			makeType: func(ns string) any {
				return &adminservice.GetNamespaceReplicationMessagesResponse{
					Messages: &replicationspb.ReplicationMessages{
						ReplicationTasks: []*replicationspb.ReplicationTask{
							{
								TaskType:     enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK,
								SourceTaskId: 123,
								Attributes: &replicationspb.ReplicationTask_NamespaceTaskAttributes{
									NamespaceTaskAttributes: nil,
								},
							},
						},
						LastRetrievedMessageId: 1234,
					},
				}
			},
		},
		{
			objName: "nil namespace info",
			makeType: func(ns string) any {
				return &adminservice.GetNamespaceReplicationMessagesResponse{
					Messages: &replicationspb.ReplicationMessages{
						ReplicationTasks: []*replicationspb.ReplicationTask{
							{
								TaskType:     enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK,
								SourceTaskId: 123,
								Attributes: &replicationspb.ReplicationTask_NamespaceTaskAttributes{
									NamespaceTaskAttributes: &replicationspb.NamespaceTaskAttributes{
										Info: nil,
									},
								},
							},
						},
					},
				}
			},
		},
		{
			objName:     "full type",
			makeType:    makeFullType,
			containsObj: true,
		},
	}
}

// testTranslateObj runs translation test variants using the given visitor and objCases
//
// HACK: The equalityAssertion function should be a compatible assertion function.
// * `require.Equal` has inconsistent behavior with generated protobuf types,
// due to internal/unexported fields.
// * `require.EqualExportedValues` works for generated protobuf types, but does not
// handle pointer cycles.
func testTranslateObj[T any](
	t *testing.T,
	visitor visitor,
	objCases []objCase,
	equalityAssertion func(t require.TestingT, exp, actual any, extra ...any),
	createMatcher func(map[string]string) T,
) {
	testcases := []struct {
		testName   string
		inputName  string
		outputName string
		mapping    map[string]string
	}{
		{
			testName:   "name changed",
			inputName:  "orig",
			outputName: "orig.cloud",
			mapping:    map[string]string{"orig": "orig.cloud"},
		},
		{
			testName:   "name unchanged",
			inputName:  "orig",
			outputName: "orig",
			mapping:    map[string]string{"other": "other.cloud"},
		},
		{
			testName:   "empty mapping",
			inputName:  "orig",
			outputName: "orig",
			mapping:    map[string]string{},
		},
		{
			testName:   "nil mapping",
			inputName:  "orig",
			outputName: "orig",
			mapping:    nil,
		},
	}
	for _, c := range objCases {
		t.Run(c.objName, func(t *testing.T) {
			for _, ts := range testcases {
				t.Run(ts.testName, func(t *testing.T) {
					input := c.makeType(ts.inputName)
					expOutput := c.makeType(ts.outputName)
					expChanged := ts.inputName != ts.outputName

					changed, err := visitor(input, createMatcher(ts.mapping))
					if len(c.expError) != 0 {
						require.ErrorContains(t, err, c.expError)
					} else {
						require.NoError(t, err)
						if c.containsObj {
							equalityAssertion(t, expOutput, input)
							require.Equal(t, expChanged, changed)
						} else {
							// input doesn't contain namespace, no change is expected.
							equalityAssertion(t, c.makeType(ts.inputName), input)
							require.False(t, changed)
						}
					}
				})
			}
		})
	}
}

func makeHistoryEventsBlobWithNamespaceField(ns string) *common.DataBlob {
	evts := []*history.HistoryEvent{
		{
			EventId:   1,
			EventType: enums.EVENT_TYPE_WORKFLOW_TASK_COMPLETED,
			Version:   1,
			TaskId:    100,
			Attributes: &history.HistoryEvent_SignalExternalWorkflowExecutionInitiatedEventAttributes{
				SignalExternalWorkflowExecutionInitiatedEventAttributes: &history.SignalExternalWorkflowExecutionInitiatedEventAttributes{
					Namespace: ns,
				},
			},
		},
		{
			EventId:   2,
			EventType: enums.EVENT_TYPE_WORKFLOW_TASK_COMPLETED,
			Version:   1,
			TaskId:    101,
			Attributes: &history.HistoryEvent_RequestCancelExternalWorkflowExecutionInitiatedEventAttributes{
				RequestCancelExternalWorkflowExecutionInitiatedEventAttributes: &history.RequestCancelExternalWorkflowExecutionInitiatedEventAttributes{
					Namespace: ns,
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
