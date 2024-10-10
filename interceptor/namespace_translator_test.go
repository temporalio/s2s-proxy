package interceptor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/command/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
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
		objName           string
		makeType          func(ns string) any
		expError          string
		containsNamespace bool
	}
)

func generateNamespaceObjCases() []objCase {
	return []objCase{
		{
			objName: "Namespace field",
			makeType: func(ns string) any {
				return &StructWithNamespaceField{Namespace: ns}
			},
			containsNamespace: true,
		},
		{
			objName: "WorkflowNamespace field",
			makeType: func(ns string) any {
				return &StructWithWorkflowNamespaceField{WorkflowNamespace: ns}
			},
			containsNamespace: true,
		},
		{
			objName: "Nested Namespace field",
			makeType: func(ns string) any {
				return &StructWithNestedNamespaceField{
					Other: "do not change",
					Nested: StructWithNamespaceField{
						Namespace: ns,
					},
				}
			},
			containsNamespace: true,
		},
		{
			objName: "list of structs",
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
			containsNamespace: true,
		},
		{
			objName: "list of ptrs",
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
			containsNamespace: true,
		},
		{
			objName: "RespondWorkflowTaskCompletedRequest",
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
			containsNamespace: true,
		},
		{
			objName: "circular pointer",
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
			containsNamespace: true,
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
			objName:           "full type",
			makeType:          makeFullType,
			containsNamespace: true,
		},
	}
}

func testTranslateNamespace(t *testing.T, objCases []objCase) {
	testcases := []struct {
		testName     string
		inputNSName  string
		outputNSName string
		mapping      map[string]string
	}{
		{
			testName:     "name changed",
			inputNSName:  "orig",
			outputNSName: "orig.cloud",
			mapping:      map[string]string{"orig": "orig.cloud"},
		},
		{
			testName:     "name unchanged",
			inputNSName:  "orig",
			outputNSName: "orig",
			mapping:      map[string]string{"other": "other.cloud"},
		},
		{
			testName:     "empty mapping",
			inputNSName:  "orig",
			outputNSName: "orig",
			mapping:      map[string]string{},
		},
		{
			testName:     "nil mapping",
			inputNSName:  "orig",
			outputNSName: "orig",
			mapping:      nil,
		},
	}
	for _, c := range objCases {
		t.Run(c.objName, func(t *testing.T) {
			for _, ts := range testcases {
				t.Run(ts.testName, func(t *testing.T) {
					input := c.makeType(ts.inputNSName)
					expOutput := c.makeType(ts.outputNSName)
					expChanged := ts.inputNSName != ts.outputNSName

					changed, err := visitNamespace(input, createNameTranslator(ts.mapping))
					if len(c.expError) != 0 {
						require.ErrorContains(t, err, c.expError)
					} else {
						require.NoError(t, err)
						if c.containsNamespace {
							require.Equal(t, expOutput, input)
							require.Equal(t, expChanged, changed)
						} else {
							// input doesn't contain namespace, no change is expected.
							require.Equal(t, c.makeType(ts.inputNSName), input)
							require.False(t, changed)
						}
					}
				})
			}
		})
	}
}

func TestTranslateNamespaceName(t *testing.T) {
	testTranslateNamespace(t, generateNamespaceObjCases())
}

func TestTranslateNamespaceReplicationMessages(t *testing.T) {
	testTranslateNamespace(t, generateNamespaceReplicationMessages())
}
