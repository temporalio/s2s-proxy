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
	case *workflowservice.ExecuteMultiOperationResponse:
		for _, item := range root.GetResponses() {
			switch oneof := item.GetResponse().(type) {
			case *workflowservice.ExecuteMultiOperationResponse_Response_StartWorkflow:
				x := oneof.StartWorkflow
				y2452265609276524908 := x.GetEagerWorkflowTask()
				y7280109239159478526 := y2452265609276524908.GetHistory()
				for _, item := range y7280109239159478526.GetEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
					// handleHistoryEvent(item)
				}
			}
		}
	case *serverreplication.ReplicationTask:
		switch oneof := root.GetAttributes().(type) {
		case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
			x := oneof.SyncWorkflowStateTaskAttributes
			y5921295452379823288 := x.GetWorkflowState()
			for _, item := range y5921295452379823288.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
				// handleHistoryEvent(item)
			}
		}
	case *serverreplication.SyncWorkflowStateTaskAttributes:
		y6160293941744092960 := root.GetWorkflowState()
		for _, item := range y6160293941744092960.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *workflowservice.StartWorkflowExecutionResponse:
		y1105924368346946996 := root.GetEagerWorkflowTask()
		y297681690037687712 := y1105924368346946996.GetHistory()
		for _, item := range y297681690037687712.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *serveradminservice.GetDLQMessagesResponse:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y8430631916927703180 := x.GetWorkflowState()
				for _, item := range y8430631916927703180.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
					// handleHistoryEvent(item)
				}
			}
		}
	case *history.HistoryEvent:
		switch oneof := root.GetAttributes().(type) {
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x := oneof.ActivityTaskStartedEventAttributes
			// repairUTF8(x)
		}
	// handleHistoryEvent(root)
	case *serveradminservice.DescribeMutableStateResponse:
		y7308793787298744425 := root.GetCacheMutableState()
		for _, item := range y7308793787298744425.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		y6418413666146703233 := root.GetWorkflowTask()
		y4346945440486438180 := y6418413666146703233.GetHistory()
		for _, item := range y4346945440486438180.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *serveradminservice.GetReplicationMessagesResponse:
		for _, val := range root.GetShardMessages() {
			for _, item := range val.GetReplicationTasks() {
				switch oneof := item.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x := oneof.SyncWorkflowStateTaskAttributes
					y4917851952680043220 := x.GetWorkflowState()
					for _, item := range y4917851952680043220.GetBufferedEvents() {
						switch oneof := item.GetAttributes().(type) {
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x := oneof.ActivityTaskStartedEventAttributes
							// repairUTF8(x)
						}
						// handleHistoryEvent(item)
					}
				}
			}
		}
	case *serverreplication.ReplicationMessages:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y7819702390493148251 := x.GetWorkflowState()
				for _, item := range y7819702390493148251.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
					// handleHistoryEvent(item)
				}
			}
		}
	case *serverreplication.VersionedTransitionArtifact:
		switch oneof := root.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y1197159871000574168 := x.GetState()
			for _, item := range y1197159871000574168.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
				// handleHistoryEvent(item)
			}
		}
	case *history.History:
		for _, item := range root.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *serverreplication.SyncWorkflowStateSnapshotAttributes:
		y9203936180825229898 := root.GetState()
		for _, item := range y9203936180825229898.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *serveradminservice.StreamWorkflowReplicationMessagesResponse:
		switch oneof := root.GetAttributes().(type) {
		case *serveradminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			x := oneof.Messages
			for _, item := range x.GetReplicationTasks() {
				switch oneof := item.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x := oneof.SyncWorkflowStateTaskAttributes
					y4127421047602945336 := x.GetWorkflowState()
					for _, item := range y4127421047602945336.GetBufferedEvents() {
						switch oneof := item.GetAttributes().(type) {
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x := oneof.ActivityTaskStartedEventAttributes
							// repairUTF8(x)
						}
						// handleHistoryEvent(item)
					}
				}
			}
		}
	case *serverreplication.WorkflowReplicationMessages:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y5805676592105902856 := x.GetWorkflowState()
				for _, item := range y5805676592105902856.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
					// handleHistoryEvent(item)
				}
			}
		}
	case *serveradminservice.GetNamespaceReplicationMessagesResponse:
		y1990967126806530708 := root.GetMessages()
		for _, item := range y1990967126806530708.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y6251677613513669015 := x.GetWorkflowState()
				for _, item := range y6251677613513669015.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
					// handleHistoryEvent(item)
				}
			}
		}
	case *workflowservice.PollWorkflowTaskQueueResponse:
		y1717708836258558942 := root.GetHistory()
		for _, item := range y1717708836258558942.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *serverpersistence.HSMCompletionCallbackArg:
		y3424499254190879284 := root.GetLastEvent()
		switch oneof := y3424499254190879284.GetAttributes().(type) {
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x := oneof.ActivityTaskStartedEventAttributes
			// repairUTF8(x)
		}
	// handleHistoryEvent(y3424499254190879284)
	case *serverreplication.SyncVersionedTransitionTaskAttributes:
		y5324290058604455849 := root.GetVersionedTransitionArtifact()
		switch oneof := y5324290058604455849.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y9180988861808914108 := x.GetState()
			for _, item := range y9180988861808914108.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
				// handleHistoryEvent(item)
			}
		}
	case *serveradminservice.GetDLQReplicationMessagesResponse:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y3768790777880045338 := x.GetWorkflowState()
				for _, item := range y3768790777880045338.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
					// handleHistoryEvent(item)
				}
			}
		}
	case *workflowservice.GetWorkflowExecutionHistoryResponse:
		y3299185320354236521 := root.GetHistory()
		for _, item := range y3299185320354236521.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *serveradminservice.SyncWorkflowStateResponse:
		y4798508749136232467 := root.GetVersionedTransitionArtifact()
		switch oneof := y4798508749136232467.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y8007183931013869036 := x.GetState()
			for _, item := range y8007183931013869036.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
				// handleHistoryEvent(item)
			}
		}
	case *serverpersistence.WorkflowMutableState:
		for _, item := range root.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *history.ActivityTaskStartedEventAttributes: // repairUTF8(root)
	case *serverhistory.TransientWorkflowTaskInfo:
		for _, item := range root.GetHistorySuffix() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	case *workflowservice.GetWorkflowExecutionHistoryReverseResponse:
		y8505037825212157467 := root.GetHistory()
		for _, item := range y8505037825212157467.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
			// handleHistoryEvent(item)
		}
	}
}
