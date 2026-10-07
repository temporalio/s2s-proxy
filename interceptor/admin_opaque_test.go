package interceptor

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpaqueKind(t *testing.T) {
	// Real paths from payload_visitor_gen.go, one per shape the visitor reports.
	cases := map[string]string{
		"StreamWorkflowReplicationMessagesResponse/Attributes/Messages/WorkflowReplicationMessages/ReplicationTasks/ReplicationTask/Attributes/SyncHsmAttributes/SyncHSMAttributes/StateMachineNode/StateMachineNode/Data":                                                                                                                                          "hsm",
		"StreamWorkflowReplicationMessagesResponse/Attributes/Messages/WorkflowReplicationMessages/ReplicationTasks/ReplicationTask/Attributes/SyncHsmAttributes/SyncHSMAttributes/StateMachineNode/StateMachineNode/Children[nexus]/op-1/Data":                                                                                                                     "hsm",
		"SyncWorkflowStateResponse/VersionedTransitionArtifact/VersionedTransitionArtifact/StateAttributes/SyncWorkflowStateSnapshotAttributes/SyncWorkflowStateSnapshotAttributes/State/WorkflowMutableState/ExecutionInfo/WorkflowExecutionInfo/StateMachineTimers/StateMachineTimerGroup/Infos/StateMachineTaskInfo/Data":                                        "hsm",
		"SyncWorkflowStateResponse/VersionedTransitionArtifact/VersionedTransitionArtifact/StateAttributes/SyncWorkflowStateSnapshotAttributes/SyncWorkflowStateSnapshotAttributes/State/WorkflowMutableState/ChasmNodes/ChasmNode/Data":                                                                                                                            "chasm",
		"SyncWorkflowStateResponse/VersionedTransitionArtifact/VersionedTransitionArtifact/StateAttributes/SyncWorkflowStateSnapshotAttributes/SyncWorkflowStateSnapshotAttributes/State/WorkflowMutableState/ChasmNodes/ChasmNode/Metadata/ChasmNodeMetadata/Attributes/ComponentAttributes/ChasmComponentAttributes/PureTasks/ChasmComponentAttributes_Task/Data": "chasm",
		"StreamWorkflowReplicationMessagesResponse/Attributes/Messages/WorkflowReplicationMessages/ReplicationTasks/ReplicationTask/Data":                                                                                                                                                                                                                           "task",
		"AddTasksRequest/Tasks/AddTasksRequest_Task/Blob":                      "task",
		"GetDLQTasksResponse/DlqTasks/HistoryDLQTask/Payload/HistoryTask/Blob": "task",
		"ImportWorkflowExecutionRequest/Token":                                 "other",
	}

	for path, want := range cases {
		require.Equal(t, want, OpaqueKind(path), path)
	}
}
