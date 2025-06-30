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
				y4381514623640899241 := x.GetEagerWorkflowTask()
				y2319230560788023725 := y4381514623640899241.GetHistory()
				for _, item := range y2319230560788023725.GetEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	case *serverreplication.SyncWorkflowStateTaskAttributes:
		y6775302133075550390 := root.GetWorkflowState()
		for _, item := range y6775302133075550390.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *serverpersistence.WorkflowMutableState:
		for _, item := range root.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *serverreplication.ReplicationTask:
		switch oneof := root.GetAttributes().(type) {
		case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
			x := oneof.SyncWorkflowStateTaskAttributes
			y7180373816048752967 := x.GetWorkflowState()
			for _, item := range y7180373816048752967.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	case *serveradminservice.GetReplicationMessagesResponse:
		for _, val := range root.GetShardMessages() {
			for _, item := range val.GetReplicationTasks() {
				switch oneof := item.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x := oneof.SyncWorkflowStateTaskAttributes
					y4220020427333400757 := x.GetWorkflowState()
					for _, item := range y4220020427333400757.GetBufferedEvents() {
						switch oneof := item.GetAttributes().(type) {
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x := oneof.ActivityTaskStartedEventAttributes
							// repairUTF8(x)
						}
					}
				}
			}
		}
	case *serveradminservice.GetDLQMessagesResponse:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y1783286350834986975 := x.GetWorkflowState()
				for _, item := range y1783286350834986975.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	case *history.HistoryEvent:
		switch oneof := root.GetAttributes().(type) {
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x := oneof.ActivityTaskStartedEventAttributes
			// repairUTF8(x)
		}
	case *serveradminservice.StreamWorkflowReplicationMessagesResponse:
		switch oneof := root.GetAttributes().(type) {
		case *serveradminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			x := oneof.Messages
			for _, item := range x.GetReplicationTasks() {
				switch oneof := item.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x := oneof.SyncWorkflowStateTaskAttributes
					y1477921973060229634 := x.GetWorkflowState()
					for _, item := range y1477921973060229634.GetBufferedEvents() {
						switch oneof := item.GetAttributes().(type) {
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x := oneof.ActivityTaskStartedEventAttributes
							// repairUTF8(x)
						}
					}
				}
			}
		}
	case *serverreplication.WorkflowReplicationMessages:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y2952610615689450218 := x.GetWorkflowState()
				for _, item := range y2952610615689450218.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	case *serverhistory.TransientWorkflowTaskInfo:
		for _, item := range root.GetHistorySuffix() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *workflowservice.PollWorkflowTaskQueueResponse:
		y6308973025250895959 := root.GetHistory()
		for _, item := range y6308973025250895959.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *serveradminservice.SyncWorkflowStateResponse:
		y3364318173093309353 := root.GetVersionedTransitionArtifact()
		switch oneof := y3364318173093309353.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y9063173129785554383 := x.GetState()
			for _, item := range y9063173129785554383.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	case *history.History:
		for _, item := range root.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *workflowservice.GetWorkflowExecutionHistoryResponse:
		y4950832020984870033 := root.GetHistory()
		for _, item := range y4950832020984870033.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *workflowservice.GetWorkflowExecutionHistoryReverseResponse:
		y1633489425124976746 := root.GetHistory()
		for _, item := range y1633489425124976746.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *serveradminservice.DescribeMutableStateResponse:
		y2180805667895753997 := root.GetCacheMutableState()
		for _, item := range y2180805667895753997.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *serveradminservice.GetDLQReplicationMessagesResponse:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y6067583730657021891 := x.GetWorkflowState()
				for _, item := range y6067583730657021891.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	case *serveradminservice.GetNamespaceReplicationMessagesResponse:
		y1979122614125749616 := root.GetMessages()
		for _, item := range y1979122614125749616.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y5970580929145528289 := x.GetWorkflowState()
				for _, item := range y5970580929145528289.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	case *serverreplication.SyncVersionedTransitionTaskAttributes:
		y6872053633400665478 := root.GetVersionedTransitionArtifact()
		switch oneof := y6872053633400665478.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y6666465556629187866 := x.GetState()
			for _, item := range y6666465556629187866.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	case *serverpersistence.HSMCompletionCallbackArg:
		y4836772665333145111 := root.GetLastEvent()
		switch oneof := y4836772665333145111.GetAttributes().(type) {
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x := oneof.ActivityTaskStartedEventAttributes
			// repairUTF8(x)
		}
	case *workflowservice.StartWorkflowExecutionResponse:
		y4092995766037843156 := root.GetEagerWorkflowTask()
		y4125453789926393483 := y4092995766037843156.GetHistory()
		for _, item := range y4125453789926393483.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *serverreplication.ReplicationMessages:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y1049941063911155444 := x.GetWorkflowState()
				for _, item := range y1049941063911155444.GetBufferedEvents() {
					switch oneof := item.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x := oneof.ActivityTaskStartedEventAttributes
						// repairUTF8(x)
					}
				}
			}
		}
	case *serverreplication.VersionedTransitionArtifact:
		switch oneof := root.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y6240117546166944997 := x.GetState()
			for _, item := range y6240117546166944997.GetBufferedEvents() {
				switch oneof := item.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x := oneof.ActivityTaskStartedEventAttributes
					// repairUTF8(x)
				}
			}
		}
	case *serverreplication.SyncWorkflowStateSnapshotAttributes:
		y3560986481423646872 := root.GetState()
		for _, item := range y3560986481423646872.GetBufferedEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *history.ActivityTaskStartedEventAttributes:
	// repairUTF8(root)
	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		y41730975532056772 := root.GetWorkflowTask()
		y4957081243830410993 := y41730975532056772.GetHistory()
		for _, item := range y4957081243830410993.GetEvents() {
			switch oneof := item.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x := oneof.ActivityTaskStartedEventAttributes
				// repairUTF8(x)
			}
		}
	case *workflowservice.ExecuteMultiOperationResponse:
		for _, item := range root.GetResponses() {
			switch oneof := item.GetResponse().(type) {
			case *workflowservice.ExecuteMultiOperationResponse_Response_StartWorkflow:
				x := oneof.StartWorkflow
				y8184714335124505542 := x.GetEagerWorkflowTask()
				y3836449114558701397 := y8184714335124505542.GetHistory()
				for _, item := range y3836449114558701397.GetEvents() {
					// handleHistoryEvent(item)
				}
			}
		}
	case *serverreplication.SyncWorkflowStateTaskAttributes:
		y1958528468701423746 := root.GetWorkflowState()
		for _, item := range y1958528468701423746.GetBufferedEvents() {
			// handleHistoryEvent(item)
		}
	case *serverpersistence.WorkflowMutableState:
		for _, item := range root.GetBufferedEvents() {
			// handleHistoryEvent(item)
		}
	case *serverreplication.ReplicationTask:
		switch oneof := root.GetAttributes().(type) {
		case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
			x := oneof.SyncWorkflowStateTaskAttributes
			y891084090995634910 := x.GetWorkflowState()
			for _, item := range y891084090995634910.GetBufferedEvents() {
				// handleHistoryEvent(item)
			}
		}
	case *serveradminservice.GetReplicationMessagesResponse:
		for _, val := range root.GetShardMessages() {
			for _, item := range val.GetReplicationTasks() {
				switch oneof := item.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x := oneof.SyncWorkflowStateTaskAttributes
					y904227325565469642 := x.GetWorkflowState()
					for _, item := range y904227325565469642.GetBufferedEvents() {
						// handleHistoryEvent(item)
					}
				}
			}
		}
	case *serveradminservice.GetDLQMessagesResponse:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y5083106542629629957 := x.GetWorkflowState()
				for _, item := range y5083106542629629957.GetBufferedEvents() {
					// handleHistoryEvent(item)
				}
			}
		}
	case *history.HistoryEvent:
	// handleHistoryEvent(root)
	case *serveradminservice.StreamWorkflowReplicationMessagesResponse:
		switch oneof := root.GetAttributes().(type) {
		case *serveradminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			x := oneof.Messages
			for _, item := range x.GetReplicationTasks() {
				switch oneof := item.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x := oneof.SyncWorkflowStateTaskAttributes
					y7528285353556085509 := x.GetWorkflowState()
					for _, item := range y7528285353556085509.GetBufferedEvents() {
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
				y355514100674157027 := x.GetWorkflowState()
				for _, item := range y355514100674157027.GetBufferedEvents() {
					// handleHistoryEvent(item)
				}
			}
		}
	case *serverhistory.TransientWorkflowTaskInfo:
		for _, item := range root.GetHistorySuffix() {
			// handleHistoryEvent(item)
		}
	case *workflowservice.PollWorkflowTaskQueueResponse:
		y1090680207884688198 := root.GetHistory()
		for _, item := range y1090680207884688198.GetEvents() {
			// handleHistoryEvent(item)
		}
	case *serveradminservice.SyncWorkflowStateResponse:
		y7315107019350377568 := root.GetVersionedTransitionArtifact()
		switch oneof := y7315107019350377568.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y3487934466159434956 := x.GetState()
			for _, item := range y3487934466159434956.GetBufferedEvents() {
				// handleHistoryEvent(item)
			}
		}
	case *history.History:
		for _, item := range root.GetEvents() {
			// handleHistoryEvent(item)
		}
	case *workflowservice.GetWorkflowExecutionHistoryResponse:
		y2492113607546540155 := root.GetHistory()
		for _, item := range y2492113607546540155.GetEvents() {
			// handleHistoryEvent(item)
		}
	case *workflowservice.GetWorkflowExecutionHistoryReverseResponse:
		y3611618091497758203 := root.GetHistory()
		for _, item := range y3611618091497758203.GetEvents() {
			// handleHistoryEvent(item)
		}
	case *serveradminservice.DescribeMutableStateResponse:
		y1518707431069528176 := root.GetCacheMutableState()
		for _, item := range y1518707431069528176.GetBufferedEvents() {
			// handleHistoryEvent(item)
		}
	case *serveradminservice.GetDLQReplicationMessagesResponse:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y1538928329839819533 := x.GetWorkflowState()
				for _, item := range y1538928329839819533.GetBufferedEvents() {
					// handleHistoryEvent(item)
				}
			}
		}
	case *serveradminservice.GetNamespaceReplicationMessagesResponse:
		y288294312528887763 := root.GetMessages()
		for _, item := range y288294312528887763.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y2069091311727612027 := x.GetWorkflowState()
				for _, item := range y2069091311727612027.GetBufferedEvents() {
					// handleHistoryEvent(item)
				}
			}
		}
	case *serverreplication.SyncVersionedTransitionTaskAttributes:
		y560784623964650358 := root.GetVersionedTransitionArtifact()
		switch oneof := y560784623964650358.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y684027704816226837 := x.GetState()
			for _, item := range y684027704816226837.GetBufferedEvents() {
				// handleHistoryEvent(item)
			}
		}
	case *serverpersistence.HSMCompletionCallbackArg:
		y8402927344975222761 := root.GetLastEvent()
	// handleHistoryEvent(y8402927344975222761)
	case *workflowservice.StartWorkflowExecutionResponse:
		y379093338183194990 := root.GetEagerWorkflowTask()
		y1629558875081263834 := y379093338183194990.GetHistory()
		for _, item := range y1629558875081263834.GetEvents() {
			// handleHistoryEvent(item)
		}
	case *serverreplication.ReplicationMessages:
		for _, item := range root.GetReplicationTasks() {
			switch oneof := item.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x := oneof.SyncWorkflowStateTaskAttributes
				y7138532579051892341 := x.GetWorkflowState()
				for _, item := range y7138532579051892341.GetBufferedEvents() {
					// handleHistoryEvent(item)
				}
			}
		}
	case *serverreplication.VersionedTransitionArtifact:
		switch oneof := root.GetStateAttributes().(type) {
		case *serverreplication.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes:
			x := oneof.SyncWorkflowStateSnapshotAttributes
			y1881623946190672642 := x.GetState()
			for _, item := range y1881623946190672642.GetBufferedEvents() {
				// handleHistoryEvent(item)
			}
		}
	case *serverreplication.SyncWorkflowStateSnapshotAttributes:
		y7724737033308497506 := root.GetState()
		for _, item := range y7724737033308497506.GetBufferedEvents() {
			// handleHistoryEvent(item)
		}
	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		y2162520260486712887 := root.GetWorkflowTask()
		y6892748496432354523 := y2162520260486712887.GetHistory()
		for _, item := range y6892748496432354523.GetEvents() {
			// handleHistoryEvent(item)
		}
	}
}
