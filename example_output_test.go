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
	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		y1 := root.GetWorkflowTask()
		y2 := y1.GetHistory()
		for _, item1 := range y2.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *serverpersistence.WorkflowMutableState:
		for _, item1 := range root.GetBufferedEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *serverhistory.TransientWorkflowTaskInfo:
		for _, item1 := range root.GetHistorySuffix() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *workflowservice.GetWorkflowExecutionHistoryResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *workflowservice.ExecuteMultiOperationResponse:
		for _, item1 := range root.GetResponses() {
			switch oneof := item1.GetResponse().(type) {
			case *workflowservice.ExecuteMultiOperationResponse_Response_StartWorkflow:
				x1 := oneof.StartWorkflow
				y1 := x1.GetEagerWorkflowTask()
				y2 := y1.GetHistory()
				for _, item2 := range y2.GetEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x2)
					}
					// handleHistoryEvent(item2)
				}
			}
		}
	case *workflowservice.PollWorkflowTaskQueueResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *serveradminservice.SyncWorkflowStateResponse:
		y1 := root.GetVersionedTransitionArtifact()
		switch oneof := y1.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x1 := oneof.SyncWorkflowStateSnapshotAttributes
			y2 := x1.GetState()
			for _, item1 := range y2.GetBufferedEvents() {
				switch oneof := item1.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x2 := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x2)
				}
				// handleHistoryEvent(item1)
			}
		}
	case *serverreplication.WorkflowReplicationMessages:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x2)
					}
					// handleHistoryEvent(item2)
				}
			}
		}
	case *workflowservice.StartWorkflowExecutionResponse:
		y1 := root.GetEagerWorkflowTask()
		y2 := y1.GetHistory()
		for _, item1 := range y2.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *serveradminservice.GetNamespaceReplicationMessagesResponse:
		y1 := root.GetMessages()
		for _, item1 := range y1.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y2 := x1.GetWorkflowState()
				for _, item2 := range y2.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x2)
					}
					// handleHistoryEvent(item2)
				}
			}
		}
	case *serveradminservice.DescribeMutableStateResponse:
		y1 := root.GetCacheMutableState()
		for _, item1 := range y1.GetBufferedEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *serverpersistence.HSMCompletionCallbackArg:
		y1 := root.GetLastEvent()
		switch oneof := y1.GetAttributes().(type) {
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x1 := oneof.ActivityTaskStartedEventAttributes
			// repairUTF8(x1)
		}
	// handleHistoryEvent(y1)
	case *serveradminservice.GetReplicationMessagesResponse:
		for _, val1 := range root.GetShardMessages() {
			for _, item1 := range val1.GetReplicationTasks() {
				switch oneof := item1.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x1 := oneof.SyncWorkflowStateTaskAttributes
					y1 := x1.GetWorkflowState()
					for _, item2 := range y1.GetBufferedEvents() {
						switch oneof := item2.GetAttributes().(type) {
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x2 := oneof.ActivityTaskStartedEventAttributes
							// repairUTF8(x2)
						}
						// handleHistoryEvent(item2)
					}
				}
			}
		}
	case *serveradminservice.GetDLQReplicationMessagesResponse:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x2)
					}
					// handleHistoryEvent(item2)
				}
			}
		}
	case *serverreplication.SyncVersionedTransitionTaskAttributes:
		y1 := root.GetVersionedTransitionArtifact()
		switch oneof := y1.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x1 := oneof.SyncWorkflowStateSnapshotAttributes
			y2 := x1.GetState()
			for _, item1 := range y2.GetBufferedEvents() {
				switch oneof := item1.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x2 := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x2)
				}
				// handleHistoryEvent(item1)
			}
		}
	case *serverreplication.ReplicationMessages:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x2)
					}
					// handleHistoryEvent(item2)
				}
			}
		}
	case *serverreplication.SyncWorkflowStateSnapshotAttributes:
		y1 := root.GetState()
		for _, item1 := range y1.GetBufferedEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *serverreplication.SyncWorkflowStateTaskAttributes:
		y1 := root.GetWorkflowState()
		for _, item1 := range y1.GetBufferedEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *serveradminservice.GetDLQMessagesResponse:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x2)
					}
					// handleHistoryEvent(item2)
				}
			}
		}
	case *history.History:
		for _, item1 := range root.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *serverreplication.VersionedTransitionArtifact:
		switch oneof := root.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x1 := oneof.SyncWorkflowStateSnapshotAttributes
			y1 := x1.GetState()
			for _, item1 := range y1.GetBufferedEvents() {
				switch oneof := item1.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x2 := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x2)
				}
				// handleHistoryEvent(item1)
			}
		}
	case *history.ActivityTaskStartedEventAttributes: // repairUTF8(root)
	case *workflowservice.GetWorkflowExecutionHistoryReverseResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x1)
			}
			// handleHistoryEvent(item1)
		}
	case *history.HistoryEvent:
		switch oneof := root.GetAttributes().(type) {
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x1 := oneof.ActivityTaskStartedEventAttributes
			// repairUTF8(x1)
		}
	// handleHistoryEvent(root)
	case *serverreplication.ReplicationTask:
		switch oneof := root.GetAttributes().(type) {
		case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
			x1 := oneof.SyncWorkflowStateTaskAttributes
			y1 := x1.GetWorkflowState()
			for _, item1 := range y1.GetBufferedEvents() {
				switch oneof := item1.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x2 := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x2)
				}
				// handleHistoryEvent(item1)
			}
		}
	case *serveradminservice.StreamWorkflowReplicationMessagesResponse:
		switch oneof := root.GetAttributes().(type) {
		case *serveradminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			x1 := oneof.Messages
			for _, item1 := range x1.GetReplicationTasks() {
				switch oneof := item1.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x2 := oneof.SyncWorkflowStateTaskAttributes
					y1 := x2.GetWorkflowState()
					for _, item2 := range y1.GetBufferedEvents() {
						switch oneof := item2.GetAttributes().(type) {
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x3 := oneof.ActivityTaskStartedEventAttributes
							// repairUTF8(x3)
						}
						// handleHistoryEvent(item2)
					}
				}
			}
		}
	}
}
