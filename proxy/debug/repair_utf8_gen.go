package debug

import (
	"github.com/temporalio/s2s-proxy/common/proto/1_22/api/history/v1"
	"github.com/temporalio/s2s-proxy/common/proto/1_22/api/workflow/v1"
	"github.com/temporalio/s2s-proxy/common/proto/1_22/api/workflowservice/v1"
	serveradminservice "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/adminservice/v1"
	serverhistory "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/history/v1"
	serverpersistence "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/persistence/v1"
	serverreplication "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/replication/v1"
)

func repairInvalidUTF8(vAny any) (ret bool) {
	switch root := vAny.(type) {
	case *history.ActivityTaskStartedEventAttributes:
		y1 := root.GetLastFailure()
		ret = ret || repairUTF8InLastFailure(y1)
	case *history.History:
		for _, item1 := range root.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y1)
			}
		}
	case *history.HistoryEvent:
		switch oneof := root.GetAttributes().(type) {
		case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
			x1 := oneof.ActivityTaskStartedEventAttributes
			y1 := x1.GetLastFailure()
			ret = ret || repairUTF8InLastFailure(y1)
		}
	case *workflow.PendingActivityInfo:
		y1 := root.GetLastFailure()
		ret = ret || repairUTF8InLastFailure(y1)
	case *workflowservice.DescribeWorkflowExecutionResponse:
		for _, item1 := range root.GetPendingActivities() {
			y1 := item1.GetLastFailure()
			ret = ret || repairUTF8InLastFailure(y1)
		}
	case *workflowservice.GetWorkflowExecutionHistoryResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y2 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y2)
			}
		}
	case *workflowservice.GetWorkflowExecutionHistoryReverseResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y2 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y2)
			}
		}
	case *workflowservice.PollWorkflowTaskQueueResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y2 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y2)
			}
		}
	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		y1 := root.GetWorkflowTask()
		y2 := y1.GetHistory()
		for _, item1 := range y2.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y3 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y3)
			}
		}
	case *workflowservice.StartWorkflowExecutionResponse:
		y1 := root.GetEagerWorkflowTask()
		y2 := y1.GetHistory()
		for _, item1 := range y2.GetEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y3 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y3)
			}
		}
	case *serveradminservice.DescribeMutableStateResponse:
		y1 := root.GetCacheMutableState()
		for _, item1 := range y1.GetBufferedEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y2 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y2)
			}
		}
	case *serveradminservice.GetDLQMessagesResponse:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y1)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						y2 := x2.GetLastFailure()
						ret = ret || repairUTF8InLastFailure(y2)
					}
				}
			}
		}
	case *serveradminservice.GetDLQReplicationMessagesResponse:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y1)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						y2 := x2.GetLastFailure()
						ret = ret || repairUTF8InLastFailure(y2)
					}
				}
			}
		}
	case *serveradminservice.GetNamespaceReplicationMessagesResponse:
		y1 := root.GetMessages()
		for _, item1 := range y1.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y2 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y2)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y2 := x1.GetWorkflowState()
				for _, item2 := range y2.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						y3 := x2.GetLastFailure()
						ret = ret || repairUTF8InLastFailure(y3)
					}
				}
			}
		}
	case *serveradminservice.GetReplicationMessagesResponse:
		for _, val1 := range root.GetShardMessages() {
			for _, item1 := range val1.GetReplicationTasks() {
				switch oneof := item1.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
					x1 := oneof.SyncActivityTaskAttributes
					y1 := x1.GetLastFailure()
					ret = ret || repairUTF8InLastFailure(y1)
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x1 := oneof.SyncWorkflowStateTaskAttributes
					y1 := x1.GetWorkflowState()
					for _, item2 := range y1.GetBufferedEvents() {
						switch oneof := item2.GetAttributes().(type) {
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x2 := oneof.ActivityTaskStartedEventAttributes
							y2 := x2.GetLastFailure()
							ret = ret || repairUTF8InLastFailure(y2)
						}
					}
				}
			}
		}
	case *serveradminservice.StreamWorkflowReplicationMessagesResponse:
		switch oneof := root.GetAttributes().(type) {
		case *serveradminservice.StreamWorkflowReplicationMessagesResponse_Messages:
			x1 := oneof.Messages
			for _, item1 := range x1.GetReplicationTasks() {
				switch oneof := item1.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
					x2 := oneof.SyncActivityTaskAttributes
					y1 := x2.GetLastFailure()
					ret = ret || repairUTF8InLastFailure(y1)
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x2 := oneof.SyncWorkflowStateTaskAttributes
					y1 := x2.GetWorkflowState()
					for _, item2 := range y1.GetBufferedEvents() {
						switch oneof := item2.GetAttributes().(type) {
						case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
							x3 := oneof.ActivityTaskStartedEventAttributes
							y2 := x3.GetLastFailure()
							ret = ret || repairUTF8InLastFailure(y2)
						}
					}
				}
			}
		}
	case *serverhistory.TransientWorkflowTaskInfo:
		for _, item1 := range root.GetHistorySuffix() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y1)
			}
		}
	case *serverpersistence.WorkflowMutableState:
		for _, item1 := range root.GetBufferedEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y1)
			}
		}
	case *serverreplication.ReplicationMessages:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y1)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						y2 := x2.GetLastFailure()
						ret = ret || repairUTF8InLastFailure(y2)
					}
				}
			}
		}
	case *serverreplication.ReplicationTask:
		switch oneof := root.GetAttributes().(type) {
		case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
			x1 := oneof.SyncActivityTaskAttributes
			y1 := x1.GetLastFailure()
			ret = ret || repairUTF8InLastFailure(y1)
		case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
			x1 := oneof.SyncWorkflowStateTaskAttributes
			y1 := x1.GetWorkflowState()
			for _, item1 := range y1.GetBufferedEvents() {
				switch oneof := item1.GetAttributes().(type) {
				case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
					x2 := oneof.ActivityTaskStartedEventAttributes
					y2 := x2.GetLastFailure()
					ret = ret || repairUTF8InLastFailure(y2)
				}
			}
		}
	case *serverreplication.SyncActivityTaskAttributes:
		y1 := root.GetLastFailure()
		ret = ret || repairUTF8InLastFailure(y1)
	case *serverreplication.SyncWorkflowStateTaskAttributes:
		y1 := root.GetWorkflowState()
		for _, item1 := range y1.GetBufferedEvents() {
			switch oneof := item1.GetAttributes().(type) {
			case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
				x1 := oneof.ActivityTaskStartedEventAttributes
				y2 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y2)
			}
		}
	case *serverreplication.WorkflowReplicationMessages:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(y1)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					switch oneof := item2.GetAttributes().(type) {
					case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
						x2 := oneof.ActivityTaskStartedEventAttributes
						y2 := x2.GetLastFailure()
						ret = ret || repairUTF8InLastFailure(y2)
					}
				}
			}
		}
	}
	return
}
