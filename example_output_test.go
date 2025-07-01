package main_test

import (
	"github.com/temporalio/s2s-proxy/common/proto/1_22/api/history/v1"
	"github.com/temporalio/s2s-proxy/common/proto/1_22/api/workflowservice/v1"
	serveradminservice "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/adminservice/v1"
	serverhistory "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/history/v1"
	serverpersistence "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/persistence/v1"
	serverreplication "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/replication/v1"
)

func VisitMessage(vAny any) {
	switch root := vAny.(type) {
	case *history.History:
		for _, item1 := range root.GetEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *history.HistoryEvent:
		repairUTF8InHistoryEvent(root)
	case *workflowservice.GetWorkflowExecutionHistoryResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *workflowservice.GetWorkflowExecutionHistoryReverseResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *workflowservice.PollWorkflowTaskQueueResponse:
		y1 := root.GetHistory()
		for _, item1 := range y1.GetEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		y1 := root.GetWorkflowTask()
		y2 := y1.GetHistory()
		for _, item1 := range y2.GetEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *workflowservice.StartWorkflowExecutionResponse:
		y1 := root.GetEagerWorkflowTask()
		y2 := y1.GetHistory()
		for _, item1 := range y2.GetEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *serveradminservice.DescribeMutableStateResponse:
		y1 := root.GetCacheMutableState()
		for _, item1 := range y1.GetBufferedEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *serveradminservice.GetDLQMessagesResponse:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					repairUTF8InHistoryEvent(item2)
				}
			}
			repairUTF8InReplicationTask(item1)
		}
	case *serveradminservice.GetDLQReplicationMessagesResponse:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					repairUTF8InHistoryEvent(item2)
				}
			}
			repairUTF8InReplicationTask(item1)
		}
	case *serveradminservice.GetNamespaceReplicationMessagesResponse:
		y1 := root.GetMessages()
		for _, item1 := range y1.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y2 := x1.GetWorkflowState()
				for _, item2 := range y2.GetBufferedEvents() {
					repairUTF8InHistoryEvent(item2)
				}
			}
			repairUTF8InReplicationTask(item1)
		}
	case *serveradminservice.GetReplicationMessagesResponse:
		for _, val1 := range root.GetShardMessages() {
			for _, item1 := range val1.GetReplicationTasks() {
				switch oneof := item1.GetAttributes().(type) {
				case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
					x1 := oneof.SyncWorkflowStateTaskAttributes
					y1 := x1.GetWorkflowState()
					for _, item2 := range y1.GetBufferedEvents() {
						repairUTF8InHistoryEvent(item2)
					}
				}
				repairUTF8InReplicationTask(item1)
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
						repairUTF8InHistoryEvent(item2)
					}
				}
				repairUTF8InReplicationTask(item1)
			}
		}
	case *serverhistory.TransientWorkflowTaskInfo:
		for _, item1 := range root.GetHistorySuffix() {
			repairUTF8InHistoryEvent(item1)
		}
	case *serverpersistence.WorkflowMutableState:
		for _, item1 := range root.GetBufferedEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *serverreplication.ReplicationMessages:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					repairUTF8InHistoryEvent(item2)
				}
			}
			repairUTF8InReplicationTask(item1)
		}
	case *serverreplication.ReplicationTask:
		switch oneof := root.GetAttributes().(type) {
		case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
			x1 := oneof.SyncWorkflowStateTaskAttributes
			y1 := x1.GetWorkflowState()
			for _, item1 := range y1.GetBufferedEvents() {
				repairUTF8InHistoryEvent(item1)
			}
		}
		repairUTF8InReplicationTask(root)
	case *serverreplication.SyncWorkflowStateTaskAttributes:
		y1 := root.GetWorkflowState()
		for _, item1 := range y1.GetBufferedEvents() {
			repairUTF8InHistoryEvent(item1)
		}
	case *serverreplication.WorkflowReplicationMessages:
		for _, item1 := range root.GetReplicationTasks() {
			switch oneof := item1.GetAttributes().(type) {
			case *serverreplication.ReplicationTask_SyncWorkflowStateTaskAttributes:
				x1 := oneof.SyncWorkflowStateTaskAttributes
				y1 := x1.GetWorkflowState()
				for _, item2 := range y1.GetBufferedEvents() {
					repairUTF8InHistoryEvent(item2)
				}
			}
			repairUTF8InReplicationTask(item1)
		}
	}
}

func repairUTF8InReplicationTask(*serverreplication.ReplicationTask) {
}

func repairUTF8InHistoryEvent(*history.HistoryEvent) {
}
