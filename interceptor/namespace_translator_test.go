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

func TestSetNamespaceBasedOnCluster(t *testing.T) {
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
		StructWithMap struct {
			Nested map[string]any
		}
	)

	permutations := []struct {
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

	cases := []struct {
		testName string
		makeType func(ns string) any
		expError string
	}{
		{
			testName: "Namespace field",
			makeType: func(ns string) any {
				return &StructWithNamespaceField{Namespace: ns}
			},
		},
		{
			testName: "WorkflowNamespace field",
			makeType: func(ns string) any {
				return &StructWithWorkflowNamespaceField{WorkflowNamespace: ns}
			},
		},
		{
			testName: "Nested Namespace field",
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
			testName: "list of structs",
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
			testName: "list of ptrs",
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
			testName: "RespondWorkflowTaskCompletedRequest",
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
			testName: "circular pointer",
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
			expError: "max depth reached",
		},
		{
			testName: "map with namespace keys",
			makeType: func(ns string) any {
				return &StructWithMap{
					Nested: map[string]any{
						"StructWithNamespaceFieldPtr": &StructWithNamespaceField{
							Namespace: ns,
						},
						"StructWithNamespaceField": StructWithNamespaceField{
							Namespace: ns,
						},
						"Namespace": ns,
						"NestedNamespace": map[string]any{
							"Namespace": ns,
							"NestedStructWithNamespaceFieldPtr": &StructWithNamespaceField{
								Namespace: ns,
							},
							"NestedStructWithNamespaceField": StructWithNamespaceField{
								Namespace: ns,
							},
						},
					},
				}
			},
		},
	}

	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			for _, perm := range permutations {
				t.Run(perm.testName, func(t *testing.T) {
					input := c.makeType(perm.inputNSName)
					expOutput := c.makeType(perm.outputNSName)
					expChanged := perm.inputNSName != perm.outputNSName

					changed, err := translateNamespace(input, perm.mapping, 0)
					if len(c.expError) != 0 {
						require.ErrorContains(t, err, c.expError)
					} else {
						require.NoError(t, err)
						require.Equal(t, expOutput, input)
						require.Equal(t, expChanged, changed)
					}
				})
			}
		})
	}
}

func TestTranslateNamespaceReplicationMessages(t *testing.T) {
	makeFullType := func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse {
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

	cases := []struct {
		testName   string
		mapping    map[string]string
		expChanged bool
		makeType   func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse
	}{
		{
			testName: "nil",
			makeType: func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse {
				return nil
			},
		},
		{
			testName: "nil messages",
			makeType: func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse {
				return &adminservice.GetNamespaceReplicationMessagesResponse{}
			},
		},
		{
			testName: "nil replication tasks",
			makeType: func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse {
				return &adminservice.GetNamespaceReplicationMessagesResponse{
					Messages: &replicationspb.ReplicationMessages{
						ReplicationTasks: nil,
					},
				}
			},
		},
		{
			testName: "nil replication tasks item",
			makeType: func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse {
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
			testName: "nil attributes",
			makeType: func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse {
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
			testName: "nil namespace task attributes",
			makeType: func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse {
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
			testName: "nil namespace info",
			makeType: func(ns string) *adminservice.GetNamespaceReplicationMessagesResponse {
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
			testName: "nil mapping",
			makeType: makeFullType,
		},
		{
			testName:   "unmapped namespace unchanged",
			mapping:    map[string]string{"other-ns": "other-ns.cloud"},
			expChanged: false,
			makeType:   makeFullType,
		},
		{
			testName:   "matched namespace changed",
			mapping:    map[string]string{"orig": "orig.cloud"},
			expChanged: true,
			makeType:   makeFullType,
		},
	}

	for _, c := range cases {
		t.Run(c.testName, func(t *testing.T) {
			input := c.makeType("orig")
			var expOutput *adminservice.GetNamespaceReplicationMessagesResponse
			if c.expChanged {
				expOutput = c.makeType("orig.cloud")
			} else {
				expOutput = c.makeType("orig")
			}
			changed := translate_GetNamespaceReplicationMessagesResponse(input, c.mapping)
			require.Equal(t, c.expChanged, changed)
			require.Equal(t, expOutput, input)
		})
	}
}
