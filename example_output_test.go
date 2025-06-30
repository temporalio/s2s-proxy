// emit - ActivityTaskStartedEventAttributes path=HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=StartWorkflowExecutionResponse/EagerWorkflowTask/PollWorkflowTaskQueueResponse/History/History/Events/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=GetWorkflowExecutionHistoryReverseResponse/History/History/Events/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=TransientWorkflowTaskInfo/HistorySuffix/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=PollWorkflowTaskQueueResponse/History/History/Events/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=ReplicationTask/Attributes/SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=VersionedTransitionArtifact/StateAttributes/SyncWorkflowStateSnapshotAttributes/State/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=GetDLQReplicationMessagesResponse/ReplicationTasks/ReplicationTask/Attributes/SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=GetWorkflowExecutionHistoryResponse/History/History/Events/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=Response/Response/StartWorkflowExecutionResponse/EagerWorkflowTask/PollWorkflowTaskQueueResponse/History/History/Events/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=ReplicationMessages/ReplicationTasks/ReplicationTask/Attributes/SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=WorkflowReplicationMessages/ReplicationTasks/ReplicationTask/Attributes/SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=GetReplicationMessagesResponse/ShardMessages/ReplicationMessages/ReplicationTasks/ReplicationTask/Attributes/SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=ExecuteMultiOperationResponse/Responses/Response/Response/StartWorkflowExecutionResponse/EagerWorkflowTask/PollWorkflowTaskQueueResponse/History/History/Events/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=HSMCompletionCallbackArg/LastEvent/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=SyncVersionedTransitionTaskAttributes/VersionedTransitionArtifact/VersionedTransitionArtifact/StateAttributes/SyncWorkflowStateSnapshotAttributes/State/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=GetDLQMessagesResponse/ReplicationTasks/ReplicationTask/Attributes/SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=StreamWorkflowReplicationMessagesResponse/Attributes/WorkflowReplicationMessages/ReplicationTasks/ReplicationTask/Attributes/SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=RespondWorkflowTaskCompletedResponse/WorkflowTask/PollWorkflowTaskQueueResponse/History/History/Events/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=History/Events/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=SyncWorkflowStateSnapshotAttributes/State/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=GetNamespaceReplicationMessagesResponse/Messages/ReplicationMessages/ReplicationTasks/ReplicationTask/Attributes/SyncWorkflowStateTaskAttributes/WorkflowState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=SyncWorkflowStateResponse/VersionedTransitionArtifact/VersionedTransitionArtifact/StateAttributes/SyncWorkflowStateSnapshotAttributes/State/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
// emit - ActivityTaskStartedEventAttributes path=DescribeMutableStateResponse/CacheMutableState/WorkflowMutableState/BufferedEvents/HistoryEvent/Attributes/ActivityTaskStartedEventAttributes
package main_test

import (
	"go.temporal.io/api/history/v1"
	"go.temporal.io/api/workflowservice/v1"
	serveradminservice "go.temporal.io/server/api/adminservice/v1"
	serverhistory "go.temporal.io/server/api/history/v1"
	serverpersistence "go.temporal.io/server/api/persistence/v1"
	serverreplication "go.temporal.io/server/api/replication/v1"
)

func VisitMessage(vAny any) {
	switch root := vAny.(type) {
	// name=HistoryEvent goname=HistoryEvent
	case *history.HistoryEvent:
		switch oneof := root.GetAttributes().(type) {
		// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
		// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x := oneof.ActivityTaskStartedEventAttributes
			// repairUTF8(x)
		}
	// name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes
	case *history.ActivityTaskStartedEventAttributes:
	// repairUTF8(root)
	// name=StartWorkflowExecutionResponse goname=StartWorkflowExecutionResponse
	case *workflowservice.StartWorkflowExecutionResponse:
		y22775235766691954 := root.GetEagerWorkflowTask()
		y5749311273663987699 := y22775235766691954.GetHistory()
		for _, item := range y5749311273663987699.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=GetWorkflowExecutionHistoryReverseResponse goname=GetWorkflowExecutionHistoryReverseResponse
	case *workflowservice.GetWorkflowExecutionHistoryReverseResponse:
		y8883653892984045906 := root.GetHistory()
		for _, item := range y8883653892984045906.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=TransientWorkflowTaskInfo goname=TransientWorkflowTaskInfo
	case *serverhistory.TransientWorkflowTaskInfo:
		for _, item := range root.GetHistorySuffix() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=WorkflowMutableState goname=WorkflowMutableState
	case *serverpersistence.WorkflowMutableState:
		for _, item := range root.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=PollWorkflowTaskQueueResponse goname=PollWorkflowTaskQueueResponse
	case *workflowservice.PollWorkflowTaskQueueResponse:
		y7577878507601886991 := root.GetHistory()
		for _, item := range y7577878507601886991.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=ReplicationTask goname=ReplicationTask
	case *serverreplication.ReplicationTask:
		switch oneof := root.GetAttributes().(type) {
		// oneof: name=sync_workflow_state_task_attributes goname=Attributes parent.Name=ReplicationTask
		// typ  : name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes parent.Name=v1
		case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
			x := oneof.SyncWorkflowStateTaskAttributes
			y1713298226227438567 := x.GetWorkflowState()
			for _, item := range y1713298226227438567.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
				// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	// name=VersionedTransitionArtifact goname=VersionedTransitionArtifact
	case *serverreplication.VersionedTransitionArtifact:
		switch oneof := root.GetStateAttributes().(type) {
		// oneof: name=sync_workflow_state_snapshot_attributes goname=StateAttributes parent.Name=VersionedTransitionArtifact
		// typ  : name=SyncWorkflowStateSnapshotAttributes goname=SyncWorkflowStateSnapshotAttributes parent.Name=v1
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y1008579546938865827 := x.GetState()
			for _, item := range y1008579546938865827.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
				// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	// name=GetDLQReplicationMessagesResponse goname=GetDLQReplicationMessagesResponse
	case *serveradminservice.GetDLQReplicationMessagesResponse:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=sync_workflow_state_task_attributes goname=Attributes parent.Name=ReplicationTask
			// typ  : name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes parent.Name=v1
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y5410410984817639213 := x.GetWorkflowState()
				for _, item := range y5410410984817639213.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
					// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	// name=GetWorkflowExecutionHistoryResponse goname=GetWorkflowExecutionHistoryResponse
	case *workflowservice.GetWorkflowExecutionHistoryResponse:
		y2585168165553088382 := root.GetHistory()
		for _, item := range y2585168165553088382.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=Response goname=Response
	case *workflowservice.Response:
		switch oneof := root.GetResponse().(type) {
		// oneof: name=start_workflow goname=Response parent.Name=Response
		// typ  : name=StartWorkflowExecutionResponse goname=StartWorkflowExecutionResponse parent.Name=v1
		case *workflowservice.ExecuteMultiOperationResponse_Response_StartWorkflow:
			x := oneof.StartWorkflowExecutionResponse
			y1628877597230433618 := x.GetEagerWorkflowTask()
			y8569249856291528770 := y1628877597230433618.GetHistory()
			for _, item := range y8569249856291528770.GetEvents() {
				switch oneof := item.GetAttributes().(type) {
				// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
				// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	// name=ReplicationMessages goname=ReplicationMessages
	case *serverreplication.ReplicationMessages:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=sync_workflow_state_task_attributes goname=Attributes parent.Name=ReplicationTask
			// typ  : name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes parent.Name=v1
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y604961218257538571 := x.GetWorkflowState()
				for _, item := range y604961218257538571.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
					// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	// name=WorkflowReplicationMessages goname=WorkflowReplicationMessages
	case *serverreplication.WorkflowReplicationMessages:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=sync_workflow_state_task_attributes goname=Attributes parent.Name=ReplicationTask
			// typ  : name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes parent.Name=v1
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y8431761642454619079 := x.GetWorkflowState()
				for _, item := range y8431761642454619079.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
					// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	// name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes
	case *serverreplication.SyncWorkflowStateTaskAttributes:
		y3782735321282930610 := root.GetWorkflowState()
		for _, item := range y3782735321282930610.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=GetReplicationMessagesResponse goname=GetReplicationMessagesResponse
	case *serveradminservice.GetReplicationMessagesResponse:
		for _, val := range root.GetShardMessages() {
			for _, item := range val.GetReplicationTasks() {
				switch oneof := item.GetAttributes().(type) {
				// oneof: name=sync_workflow_state_task_attributes goname=Attributes parent.Name=ReplicationTask
				// typ  : name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes parent.Name=v1
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x := oneof.SyncWorkflowStateTaskAttributes
					y4944803433367137477 := x.GetWorkflowState()
					for _, item := range y4944803433367137477.GetBufferedEvents() {
						switch oneof := item.GetAttributes().(type) {
						// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
						// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x := oneof.ActivityTaskStartedEventAttributes
							// repairUTF8(x)
						}
					}
				}
			}
		}
	// name=ExecuteMultiOperationResponse goname=ExecuteMultiOperationResponse
	case *workflowservice.ExecuteMultiOperationResponse:
		for _, item := range root.GetResponses() {
			switch oneof := item.GetResponse().(type) {
			// oneof: name=start_workflow goname=Response parent.Name=Response
			// typ  : name=StartWorkflowExecutionResponse goname=StartWorkflowExecutionResponse parent.Name=v1
			case *workflowservice.ExecuteMultiOperationResponse_Response_StartWorkflow:
				x := oneof.StartWorkflowExecutionResponse
				y5573731278446339347 := x.GetEagerWorkflowTask()
				y6656184001724219969 := y5573731278446339347.GetHistory()
				for _, item := range y6656184001724219969.GetEvents() {
					switch oneof := item.GetAttributes().(type) {
					// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
					// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	// name=HSMCompletionCallbackArg goname=HSMCompletionCallbackArg
	case *serverpersistence.HSMCompletionCallbackArg:
		y2438555495146797689 := root.GetLastEvent()
		switch oneof := y2438555495146797689.GetAttributes().(type) {
		// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
		// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x := oneof.ActivityTaskStartedEventAttributes
			// repairUTF8(x)
		}
	// name=SyncVersionedTransitionTaskAttributes goname=SyncVersionedTransitionTaskAttributes
	case *serverreplication.SyncVersionedTransitionTaskAttributes:
		y7418693646060097692 := root.GetVersionedTransitionArtifact()
		switch oneof := y7418693646060097692.GetStateAttributes().(type) {
		// oneof: name=sync_workflow_state_snapshot_attributes goname=StateAttributes parent.Name=VersionedTransitionArtifact
		// typ  : name=SyncWorkflowStateSnapshotAttributes goname=SyncWorkflowStateSnapshotAttributes parent.Name=v1
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y7226682955459726779 := x.GetState()
			for _, item := range y7226682955459726779.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
				// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	// name=GetDLQMessagesResponse goname=GetDLQMessagesResponse
	case *serveradminservice.GetDLQMessagesResponse:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=sync_workflow_state_task_attributes goname=Attributes parent.Name=ReplicationTask
			// typ  : name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes parent.Name=v1
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y8153247295338291959 := x.GetWorkflowState()
				for _, item := range y8153247295338291959.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
					// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	// name=StreamWorkflowReplicationMessagesResponse goname=StreamWorkflowReplicationMessagesResponse
	case *serveradminservice.StreamWorkflowReplicationMessagesResponse:
		switch oneof := root.GetAttributes().(type) {
		// oneof: name=messages goname=Attributes parent.Name=StreamWorkflowReplicationMessagesResponse
		// typ  : name=WorkflowReplicationMessages goname=WorkflowReplicationMessages parent.Name=v1
		case *serveradminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			x := oneof.WorkflowReplicationMessages
			for _, item := range x.GetReplicationTasks() {
				switch oneof := item.GetAttributes().(type) {
				// oneof: name=sync_workflow_state_task_attributes goname=Attributes parent.Name=ReplicationTask
				// typ  : name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes parent.Name=v1
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x := oneof.SyncWorkflowStateTaskAttributes
					y2935944630906514776 := x.GetWorkflowState()
					for _, item := range y2935944630906514776.GetBufferedEvents() {
						switch oneof := item.GetAttributes().(type) {
						// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
						// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x := oneof.ActivityTaskStartedEventAttributes
							// repairUTF8(x)
						}
					}
				}
			}
		}
	// name=RespondWorkflowTaskCompletedResponse goname=RespondWorkflowTaskCompletedResponse
	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		y962741622106513331 := root.GetWorkflowTask()
		y3664382378305204310 := y962741622106513331.GetHistory()
		for _, item := range y3664382378305204310.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=History goname=History
	case *history.History:
		for _, item := range root.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=SyncWorkflowStateSnapshotAttributes goname=SyncWorkflowStateSnapshotAttributes
	case *serverreplication.SyncWorkflowStateSnapshotAttributes:
		y1754430560650612685 := root.GetState()
		for _, item := range y1754430560650612685.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	// name=GetNamespaceReplicationMessagesResponse goname=GetNamespaceReplicationMessagesResponse
	case *serveradminservice.GetNamespaceReplicationMessagesResponse:
		y9056885728854800072 := root.GetMessages()
		for _, item := range y9056885728854800072.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=sync_workflow_state_task_attributes goname=Attributes parent.Name=ReplicationTask
			// typ  : name=SyncWorkflowStateTaskAttributes goname=SyncWorkflowStateTaskAttributes parent.Name=v1
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y3073419771097398575 := x.GetWorkflowState()
				for _, item := range y3073419771097398575.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
					// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	// name=SyncWorkflowStateResponse goname=SyncWorkflowStateResponse
	case *serveradminservice.SyncWorkflowStateResponse:
		y6049692165119747149 := root.GetVersionedTransitionArtifact()
		switch oneof := y6049692165119747149.GetStateAttributes().(type) {
		// oneof: name=sync_workflow_state_snapshot_attributes goname=StateAttributes parent.Name=VersionedTransitionArtifact
		// typ  : name=SyncWorkflowStateSnapshotAttributes goname=SyncWorkflowStateSnapshotAttributes parent.Name=v1
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y3679005459185820567 := x.GetState()
			for _, item := range y3679005459185820567.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
				// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	// name=DescribeMutableStateResponse goname=DescribeMutableStateResponse
	case *serveradminservice.DescribeMutableStateResponse:
		y8562557119113800320 := root.GetCacheMutableState()
		for _, item := range y8562557119113800320.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			// oneof: name=activity_task_started_event_attributes goname=Attributes parent.Name=HistoryEvent
			// typ  : name=ActivityTaskStartedEventAttributes goname=ActivityTaskStartedEventAttributes parent.Name=v1
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	}
}
