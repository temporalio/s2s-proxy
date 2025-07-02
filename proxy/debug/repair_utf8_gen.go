package debug

import (
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/common/proto/1_22/api/history/v1"
	"github.com/temporalio/s2s-proxy/common/proto/1_22/api/workflow/v1"
	"github.com/temporalio/s2s-proxy/common/proto/1_22/api/workflowservice/v1"
	serveradminservice "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/adminservice/v1"
	serverhistory "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/history/v1"
	serverpersistence "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/persistence/v1"
	serverreplication "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/replication/v1"
)

func repairHistoryEvent(logger log.Logger, root *history.HistoryEvent) bool {
	switch oneof := root.GetAttributes().(type) {
	case *history.HistoryEvent_ActivityTaskStartedEventAttributes:
		x1 := oneof.ActivityTaskStartedEventAttributes
		y1 := x1.GetLastFailure()
		return repairUTF8InLastFailure(logger, y1)
	case *history.HistoryEvent_ActivityTaskFailedEventAttributes:
		x1 := oneof.ActivityTaskFailedEventAttributes
		y1 := x1.GetFailure()
		return repairUTF8InLastFailure(logger, y1)
	case *history.HistoryEvent_ActivityTaskTimedOutEventAttributes:
		x1 := oneof.ActivityTaskTimedOutEventAttributes
		y1 := x1.GetFailure()
		return repairUTF8InLastFailure(logger, y1)
	}

	return false
}

func repairInvalidUTF8(logger log.Logger, vAny any) (ret bool) {
	switch root := vAny.(type) {
	case *history.ActivityTaskStartedEventAttributes:
		y1 := root.GetLastFailure()
		ret = ret || repairUTF8InLastFailure(logger, y1)

	case *history.ActivityTaskFailedEventAttributes:
		y1 := root.GetFailure()
		ret = ret || repairUTF8InLastFailure(logger, y1)

	case *history.ActivityTaskTimedOutEventAttributes:
		y1 := root.GetFailure()
		ret = ret || repairUTF8InLastFailure(logger, y1)

	case *history.History:
		for _, item1 := range root.GetEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *history.HistoryEvent:
		ret = ret || repairHistoryEvent(logger, root)
	case *workflow.PendingActivityInfo:
		y1 := root.GetLastFailure()
		ret = ret || repairUTF8InLastFailure(logger, y1)
	case *workflowservice.DescribeWorkflowExecutionResponse:
		for _, item1 := range root.GetPendingActivities() {
			y1 := item1.GetLastFailure()
			ret = ret || repairUTF8InLastFailure(logger, y1)
		}
	case *workflowservice.GetWorkflowExecutionHistoryResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *workflowservice.GetWorkflowExecutionHistoryReverseResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *workflowservice.PollWorkflowTaskQueueResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		y1 := root.GetWorkflowTask()
		y2 := y1.GetHistory()
		for _, item1 := range y2.GetEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *workflowservice.StartWorkflowExecutionResponse:
		y1 := root.GetEagerWorkflowTask()
		y2 := y1.GetHistory()
		for _, item1 := range y2.GetEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *serveradminservice.DescribeMutableStateResponse:
		y1 := root.GetCacheMutableState()
		for _, item1 := range y1.GetBufferedEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *serveradminservice.GetDLQMessagesResponse:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(logger, y1)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					ret = ret || repairHistoryEvent(logger, item2)
				}
			}
		}
	case *serveradminservice.GetDLQReplicationMessagesResponse:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(logger, y1)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					ret = ret || repairHistoryEvent(logger, item2)
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
				ret = ret || repairUTF8InLastFailure(logger, y2)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y2 := x1.GetWorkflowState()
				for _, item2 := range y2.GetBufferedEvents() {
					ret = ret || repairHistoryEvent(logger, item2)
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
					ret = ret || repairUTF8InLastFailure(logger, y1)
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x1 := oneof.SyncWorkflowStateTaskAttributes
					y1 := x1.GetWorkflowState()
					for _, item2 := range y1.GetBufferedEvents() {
						ret = ret || repairHistoryEvent(logger, item2)
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
					ret = ret || repairUTF8InLastFailure(logger, y1)
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x2 := oneof.SyncWorkflowStateTaskAttributes
					y1 := x2.GetWorkflowState()
					for _, item2 := range y1.GetBufferedEvents() {
						ret = ret || repairHistoryEvent(logger, item2)
					}
				}
			}
		}
	case *serverhistory.TransientWorkflowTaskInfo:
		for _, item1 := range root.GetHistorySuffix() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *serverpersistence.WorkflowMutableState:
		for _, item1 := range root.GetBufferedEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *serverreplication.ReplicationMessages:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(logger, y1)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					ret = ret || repairHistoryEvent(logger, item2)
				}
			}
		}
	case *serverreplication.ReplicationTask:
		switch oneof := root.GetAttributes().(type) {
		case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
			x1 := oneof.SyncActivityTaskAttributes
			y1 := x1.GetLastFailure()
			ret = ret || repairUTF8InLastFailure(logger, y1)
		case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
			x1 := oneof.SyncWorkflowStateTaskAttributes
			y1 := x1.GetWorkflowState()
			for _, item1 := range y1.GetBufferedEvents() {
				ret = ret || repairHistoryEvent(logger, item1)
			}
		}
	case *serverreplication.SyncActivityTaskAttributes:
		y1 := root.GetLastFailure()
		ret = ret || repairUTF8InLastFailure(logger, y1)
	case *serverreplication.SyncWorkflowStateTaskAttributes:
		y1 := root.GetWorkflowState()
		for _, item1 := range y1.GetBufferedEvents() {
			ret = ret || repairHistoryEvent(logger, item1)
		}
	case *serverreplication.WorkflowReplicationMessages:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncActivityTaskAttributes:
				x1 := oneof.SyncActivityTaskAttributes
				y1 := x1.GetLastFailure()
				ret = ret || repairUTF8InLastFailure(logger, y1)
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					ret = ret || repairHistoryEvent(logger, item2)
				}
			}
		}
	}
	return
}
