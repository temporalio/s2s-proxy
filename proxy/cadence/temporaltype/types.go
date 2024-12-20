package temporaltype

import (
	"fmt"
	"github.com/gogo/protobuf/types"
	adminv1 "github.com/uber/cadence-idl/go/proto/admin/v1"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/command/v1"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/query/v1"
	"go.temporal.io/api/replication/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/server/api/adminservice/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	repication "go.temporal.io/server/api/replication/v1"
	servercommon "go.temporal.io/server/common"
	"golang.org/x/net/context"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func PollWorkflowTaskQueueRequest(req *cadence.PollForDecisionTaskRequest) *workflowservice.PollWorkflowTaskQueueRequest {
	if req == nil {
		return nil
	}

	return &workflowservice.PollWorkflowTaskQueueRequest{
		Namespace:      req.GetDomain(),
		TaskQueue:      TaskQueue(req.GetTaskList()),
		Identity:       req.GetIdentity(),
		BinaryChecksum: req.GetBinaryChecksum(),
	}
}

func TaskQueue(taskList *cadence.TaskList) *taskqueue.TaskQueue {
	if taskList == nil {
		return nil
	}

	return &taskqueue.TaskQueue{
		Name: taskList.GetName(),
		Kind: TaskQueueKind(taskList.GetKind()),
	}
}

func TaskQueueKind(kind cadence.TaskListKind) enums.TaskQueueKind {
	switch kind {
	case cadence.TaskListKind_TASK_LIST_KIND_NORMAL:
		return enums.TASK_QUEUE_KIND_NORMAL
	case cadence.TaskListKind_TASK_LIST_KIND_STICKY:
		return enums.TASK_QUEUE_KIND_STICKY
	case cadence.TaskListKind_TASK_LIST_KIND_INVALID:
		return enums.TASK_QUEUE_KIND_UNSPECIFIED
	default:
		return enums.TASK_QUEUE_KIND_UNSPECIFIED
	}
}

func RespondWorkflowTaskCompletedRequest(
	request *cadence.RespondDecisionTaskCompletedRequest,
	wsClient workflowservice.WorkflowServiceClient,
) *workflowservice.RespondWorkflowTaskCompletedRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondWorkflowTaskCompletedRequest{
		TaskToken:                  request.GetTaskToken(),
		Commands:                   Commands(request.GetDecisions(), wsClient, request.GetTaskToken()),
		Identity:                   request.GetIdentity(),
		StickyAttributes:           StickyAttributes(request.GetStickyAttributes()),
		ReturnNewWorkflowTask:      request.GetReturnNewDecisionTask(),
		ForceCreateNewWorkflowTask: request.GetForceCreateNewDecisionTask(),
		BinaryChecksum:             request.GetBinaryChecksum(),
		QueryResults:               QueryResults(request.GetQueryResults()),
		//Namespace:                  "",
		//WorkerVersionStamp:         nil,
		//Messages:                   nil,
		//SdkMetadata:                nil,
		//MeteringMetadata:           nil,
	}
}

func QueryResults(results map[string]*cadence.WorkflowQueryResult) map[string]*query.WorkflowQueryResult {
	return nil
}

func StickyAttributes(attributes *cadence.StickyExecutionAttributes) *taskqueue.StickyExecutionAttributes {
	return nil
}

func Commands(decisions []*cadence.Decision, wsClient workflowservice.WorkflowServiceClient, taskToken []byte) []*command.Command {
	if decisions == nil {
		return nil
	}

	commands := make([]*command.Command, len(decisions))
	for i, decision := range decisions {
		commands[i] = Command(decision, wsClient, taskToken)
	}
	return commands
}

func Command(
	decision *cadence.Decision,
	wsClient workflowservice.WorkflowServiceClient,
	taskToken []byte,
) *command.Command {
	if decision == nil {
		return nil
	}

	c := &command.Command{}

	switch decision.Attributes.(type) {
	case *cadence.Decision_ScheduleActivityTaskDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_SCHEDULE_ACTIVITY_TASK
		c.Attributes = ScheduleActivityTaskCommandAttributes(decision.GetScheduleActivityTaskDecisionAttributes())
	case *cadence.Decision_StartTimerDecisionAttributes:
	case *cadence.Decision_CompleteWorkflowExecutionDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_COMPLETE_WORKFLOW_EXECUTION
		c.Attributes = CompleteWorkflowExecutionCommandAttributes(decision.GetCompleteWorkflowExecutionDecisionAttributes())
	case *cadence.Decision_FailWorkflowExecutionDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_FAIL_WORKFLOW_EXECUTION
		c.Attributes = FailWorkflowExecutionCommandAttributes(decision.GetFailWorkflowExecutionDecisionAttributes())
	case *cadence.Decision_RequestCancelActivityTaskDecisionAttributes:
		c.CommandType = enums.COMMAND_TYPE_REQUEST_CANCEL_ACTIVITY_TASK
		c.Attributes = RequestCancelActivityTaskCommandAttributes(decision.GetRequestCancelActivityTaskDecisionAttributes(), wsClient, taskToken)
	case *cadence.Decision_CancelTimerDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_CancelWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_RequestCancelExternalWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_RecordMarkerDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_ContinueAsNewWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_StartChildWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_SignalExternalWorkflowExecutionDecisionAttributes:
		panic("not implemented" + decision.String())
	case *cadence.Decision_UpsertWorkflowSearchAttributesDecisionAttributes:
		panic("not implemented" + decision.String())
	default:
		panic(fmt.Sprintf("not implemented decision type conversion: %T", decision.Attributes))
	}

	return c
}

func RequestCancelActivityTaskCommandAttributes(
	attributes *cadence.RequestCancelActivityTaskDecisionAttributes,
	wsClient workflowservice.WorkflowServiceClient,
	taskToken []byte,
) *command.Command_RequestCancelActivityTaskCommandAttributes {
	activityID := getScheduledEventIDByActivityID(attributes.GetActivityId(), wsClient, taskToken)

	return &command.Command_RequestCancelActivityTaskCommandAttributes{
		RequestCancelActivityTaskCommandAttributes: &command.RequestCancelActivityTaskCommandAttributes{
			ScheduledEventId: activityID,
		},
	}
}

func getScheduledEventIDByActivityID(activityID string, wsClient workflowservice.WorkflowServiceClient, token []byte) int64 {
	taskToken, err := servercommon.NewProtoTaskTokenSerializer().Deserialize(token)
	if err != nil {
		panic(fmt.Sprintf("failed to deserialize task token: %v", err))
	}

	descNSResq, err := wsClient.DescribeNamespace(context.Background(), &workflowservice.DescribeNamespaceRequest{
		Id: taskToken.GetNamespaceId(),
	})
	if err != nil {
		panic(fmt.Sprintf("failed to describe namespace: %v", err))
	}

	getHistoryResp, err := wsClient.GetWorkflowExecutionHistory(context.Background(), &workflowservice.GetWorkflowExecutionHistoryRequest{
		Namespace: descNSResq.GetNamespaceInfo().GetName(),
		Execution: &common.WorkflowExecution{
			WorkflowId: taskToken.GetWorkflowId(),
			RunId:      taskToken.GetRunId(),
		},
		MaximumPageSize: 0,
		NextPageToken:   nil,
		WaitNewEvent:    false,
		SkipArchival:    true,
	})

	if err != nil {
		panic(fmt.Sprintf("failed to get workflow history: %v", err))
	}

	events := getHistoryResp.GetHistory().GetEvents()
	for _, event := range events {
		if event.GetEventType() == enums.EVENT_TYPE_ACTIVITY_TASK_SCHEDULED {
			attributes := event.GetActivityTaskScheduledEventAttributes()
			if attributes.GetActivityId() == activityID {
				return event.GetEventId()
			}
		}
	}

	panic(fmt.Sprintf("activity not found: %s", activityID))
}

func FailWorkflowExecutionCommandAttributes(
	attributes *cadence.FailWorkflowExecutionDecisionAttributes,
) *command.Command_FailWorkflowExecutionCommandAttributes {
	return &command.Command_FailWorkflowExecutionCommandAttributes{
		FailWorkflowExecutionCommandAttributes: &command.FailWorkflowExecutionCommandAttributes{
			Failure: Failure(attributes.GetFailure()),
		},
	}
}

func CompleteWorkflowExecutionCommandAttributes(
	attributes *cadence.CompleteWorkflowExecutionDecisionAttributes,
) *command.Command_CompleteWorkflowExecutionCommandAttributes {
	return &command.Command_CompleteWorkflowExecutionCommandAttributes{
		CompleteWorkflowExecutionCommandAttributes: &command.CompleteWorkflowExecutionCommandAttributes{
			Result: Payload(attributes.GetResult()),
		},
	}
}

func ScheduleActivityTaskCommandAttributes(
	attributes *cadence.ScheduleActivityTaskDecisionAttributes,
) *command.Command_ScheduleActivityTaskCommandAttributes {
	return &command.Command_ScheduleActivityTaskCommandAttributes{
		ScheduleActivityTaskCommandAttributes: &command.ScheduleActivityTaskCommandAttributes{
			ActivityId: attributes.GetActivityId(),
			ActivityType: &common.ActivityType{
				Name: attributes.GetActivityType().GetName(),
			},
			TaskQueue:              TaskQueue(attributes.GetTaskList()),
			Header:                 nil,
			Input:                  Payload(attributes.GetInput()),
			ScheduleToCloseTimeout: Duration(attributes.GetScheduleToCloseTimeout()),
			ScheduleToStartTimeout: Duration(attributes.GetScheduleToStartTimeout()),
			StartToCloseTimeout:    Duration(attributes.GetStartToCloseTimeout()),
			HeartbeatTimeout:       Duration(attributes.GetHeartbeatTimeout()),
			RetryPolicy:            RetryPolicy(attributes.GetRetryPolicy()),
			RequestEagerExecution:  false,
			UseWorkflowBuildId:     false,
		},
	}
}

func RetryPolicy(policy *cadence.RetryPolicy) *common.RetryPolicy {
	if policy == nil {
		return nil
	}

	return &common.RetryPolicy{
		InitialInterval:        Duration(policy.GetInitialInterval()),
		BackoffCoefficient:     policy.GetBackoffCoefficient(),
		MaximumInterval:        Duration(policy.GetMaximumInterval()),
		MaximumAttempts:        policy.GetMaximumAttempts(),
		NonRetryableErrorTypes: policy.GetNonRetryableErrorReasons(),
	}
}

func Payload(input *cadence.Payload) *common.Payloads {
	if input == nil {
		return nil
	}

	if payloads, err := converter.GetDefaultDataConverter().ToPayloads(input.GetData()); err != nil {
		panic(fmt.Sprintf("failed to convert cadence payload to temporal: %v", err))
	} else {
		return payloads
	}
}

func Duration(d *types.Duration) *durationpb.Duration {
	if d == nil {
		return nil
	}

	return &durationpb.Duration{
		Seconds: d.GetSeconds(),
		Nanos:   d.GetNanos(),
	}
}

func PollActivityTaskQueueRequest(request *cadence.PollForActivityTaskRequest) *workflowservice.PollActivityTaskQueueRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.PollActivityTaskQueueRequest{
		Namespace: request.GetDomain(),
		TaskQueue: TaskQueue(request.GetTaskList()),
		Identity:  request.GetIdentity(),
		//TaskQueueMetadata:         nil,
		//WorkerVersionCapabilities: nil,
	}
}

func RespondActivityTaskCompletedRequest(request *cadence.RespondActivityTaskCompletedRequest) *workflowservice.RespondActivityTaskCompletedRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondActivityTaskCompletedRequest{
		TaskToken: request.GetTaskToken(),
		Result:    Payload(request.GetResult()),
		Identity:  request.GetIdentity(),
		//Namespace:     "",
		//WorkerVersion: nil,
	}

}

func RespondActivityTaskFailedRequest(request *cadence.RespondActivityTaskFailedRequest) *workflowservice.RespondActivityTaskFailedRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondActivityTaskFailedRequest{
		TaskToken: request.GetTaskToken(),
		Failure:   Failure(request.GetFailure()),
		Identity:  request.GetIdentity(),
		//Namespace:     "",
		//LastHeartbeatDetails:,
		//WorkerVersion: nil,
	}
}

func Failure(f *cadence.Failure) *failure.Failure {
	if f == nil {
		return nil
	}

	return &failure.Failure{
		Message: f.GetReason(),
		//Source:            "",
		//StackTrace:        "",
		//EncodedAttributes: nil,
		//Cause:             nil,
		//FailureInfo:       nil,
	}
}

func RecordActivityTaskHeartbeatRequest(
	request *cadence.RecordActivityTaskHeartbeatRequest,
) *workflowservice.RecordActivityTaskHeartbeatRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RecordActivityTaskHeartbeatRequest{
		TaskToken: request.GetTaskToken(),
		Details:   Payload(request.GetDetails()),
		Identity:  request.GetIdentity(),
		//Namespace: "",
	}
}

func ResetStickyTaskQueueRequest(
	request *cadence.ResetStickyTaskListRequest,
) *workflowservice.ResetStickyTaskQueueRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.ResetStickyTaskQueueRequest{
		Namespace: request.GetDomain(),
		Execution: WorkflowExecution(request.GetWorkflowExecution()),
	}
}

func WorkflowExecution(execution *cadence.WorkflowExecution) *common.WorkflowExecution {
	if execution == nil {
		return nil
	}

	return &common.WorkflowExecution{
		WorkflowId: execution.GetWorkflowId(),
		RunId:      execution.GetRunId(),
	}
}

func RespondActivityTaskCanceledRequest(request *cadence.RespondActivityTaskCanceledRequest) *workflowservice.RespondActivityTaskCanceledRequest {
	if request == nil {
		return nil
	}

	return &workflowservice.RespondActivityTaskCanceledRequest{
		TaskToken: request.GetTaskToken(),
		Details:   Payload(request.GetDetails()),
		Identity:  request.GetIdentity(),
		//Namespace:     "",
		//WorkerVersion: nil,
	}
}

func GetReplicationMessagesRequest(request *adminv1.GetReplicationMessagesRequest) *adminservice.GetReplicationMessagesRequest {
	if request == nil {
		return nil
	}

	return &adminservice.GetReplicationMessagesRequest{
		Tokens:      ReplicationToken(request.GetTokens()),
		ClusterName: request.GetClusterName(),
	}
}

func ReplicationToken(tokens []*adminv1.ReplicationToken) []*repication.ReplicationToken {
	if tokens == nil {
		return nil
	}

	result := make([]*repication.ReplicationToken, len(tokens))
	for i, token := range tokens {
		result[i] = &repication.ReplicationToken{
			ShardId:                token.GetShardId(),
			LastRetrievedMessageId: token.GetLastRetrievedMessageId(),
			LastProcessedMessageId: token.GetLastProcessedMessageId(),
			//LastProcessedVisibilityTime: nil,
		}
	}
	return result
}

func GetReplicationMessagesResponse(resp *adminv1.GetReplicationMessagesResponse) *adminservice.GetReplicationMessagesResponse {
	if resp == nil {
		return nil
	}

	messages := make(map[int32]*repication.ReplicationMessages, len(resp.GetShardMessages()))
	for i, m := range resp.GetShardMessages() {
		messages[i] = &repication.ReplicationMessages{
			ReplicationTasks:       ReplicationTasks(m.GetReplicationTasks()),
			LastRetrievedMessageId: m.GetLastRetrievedMessageId(),
			HasMore:                m.GetHasMore(),
			SyncShardStatus:        SyncShardStatus(m.GetSyncShardStatus()),
		}
	}

	return &adminservice.GetReplicationMessagesResponse{
		ShardMessages: messages,
	}
}

func ReplicationTasks(tasks []*adminv1.ReplicationTask) []*repication.ReplicationTask {
	if tasks == nil {
		return nil
	}

	result := make([]*repication.ReplicationTask, len(tasks))
	for i, task := range tasks {
		result[i] = ReplicationTask(task)
	}
	return result
}

func ReplicationTask(t *adminv1.ReplicationTask) *repication.ReplicationTask {
	task := &repication.ReplicationTask{
		SourceTaskId:   t.GetSourceTaskId(),
		VisibilityTime: Timestamp(t.GetCreationTime()),
	}

	switch t.GetTaskType() {
	case adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_HISTORY:
		task.TaskType = enumsspb.REPLICATION_TASK_TYPE_HISTORY_TASK
		task.Attributes = &repication.ReplicationTask_HistoryTaskAttributes{
			HistoryTaskAttributes: &repication.HistoryTaskAttributes{},
		}
	case adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_SYNC_SHARD_STATUS:
		task.TaskType = enumsspb.REPLICATION_TASK_TYPE_SYNC_SHARD_STATUS_TASK
	case adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_DOMAIN:
		task.TaskType = enumsspb.REPLICATION_TASK_TYPE_NAMESPACE_TASK
		task.Attributes = NamespaceTaskAttributes(t.GetDomainTaskAttributes())
	case adminv1.ReplicationTaskType_REPLICATION_TASK_TYPE_SYNC_ACTIVITY:
		task.TaskType = enumsspb.REPLICATION_TASK_TYPE_SYNC_ACTIVITY_TASK
	default:
		panic(fmt.Sprintf("unknown Cadence replication task type: %v", t.GetTaskType()))
	}

	return task
}

func NamespaceTaskAttributes(attributes *adminv1.DomainTaskAttributes) *repication.ReplicationTask_NamespaceTaskAttributes {
	return &repication.ReplicationTask_NamespaceTaskAttributes{
		NamespaceTaskAttributes: &repication.NamespaceTaskAttributes{
			NamespaceOperation: NamespaceOperation(attributes.GetDomainOperation()),
			Id:                 attributes.GetId(),
			Info:               NamespaceInfo(attributes.GetDomain()),
			Config:             NamespaceConfig(attributes.GetDomain()),
			ReplicationConfig:  ReplicationConfig(attributes),
			ConfigVersion:      attributes.GetConfigVersion(),
			FailoverVersion:    attributes.GetFailoverVersion(),
			//FailoverHistory:       nil,
			//NexusOutgoingServices: nil,
		},
	}
}

func ReplicationConfig(attributes *adminv1.DomainTaskAttributes) *replication.NamespaceReplicationConfig {
	var clusters []*replication.ClusterReplicationConfig
	for _, cluster := range attributes.GetDomain().GetClusters() {
		clusters = append(clusters, &replication.ClusterReplicationConfig{
			ClusterName: cluster.GetClusterName(),
		})
	}

	return &replication.NamespaceReplicationConfig{
		ActiveClusterName: attributes.GetDomain().GetActiveClusterName(),
		Clusters:          clusters,
		State:             enums.REPLICATION_STATE_NORMAL,
	}
}

func NamespaceConfig(domain *cadence.Domain) *namespace.NamespaceConfig {
	if domain == nil {
		return nil
	}

	return &namespace.NamespaceConfig{
		WorkflowExecutionRetentionTtl: Duration(domain.GetWorkflowExecutionRetentionPeriod()),
		BadBinaries:                   BadBinaries(domain.GetBadBinaries()),
		HistoryArchivalState:          enums.ArchivalState(domain.GetHistoryArchivalStatus()),
		HistoryArchivalUri:            domain.GetHistoryArchivalUri(),
		VisibilityArchivalState:       enums.ArchivalState(domain.GetVisibilityArchivalStatus()),
		VisibilityArchivalUri:         domain.GetVisibilityArchivalUri(),
		//CustomSearchAttributeAliases:  nil,
	}
}

func BadBinaries(binaries *cadence.BadBinaries) *namespace.BadBinaries {
	if binaries == nil {
		return nil
	}

	badBinaries := make(map[string]*namespace.BadBinaryInfo)
	for k, v := range binaries.GetBinaries() {
		badBinaries[k] = &namespace.BadBinaryInfo{
			Reason:     v.GetReason(),
			Operator:   v.GetOperator(),
			CreateTime: Timestamp(v.GetCreatedTime()),
		}
	}

	return &namespace.BadBinaries{
		Binaries: badBinaries,
	}
}

func NamespaceInfo(domain *cadence.Domain) *namespace.NamespaceInfo {
	if domain == nil {
		return nil
	}

	return &namespace.NamespaceInfo{
		Name:        domain.GetName(),
		State:       enums.NamespaceState(domain.GetStatus()),
		Description: domain.GetDescription(),
		OwnerEmail:  domain.GetOwnerEmail(),
		Data:        domain.GetData(),
		Id:          domain.GetId(),
		//Capabilities:      nil,
		//SupportsSchedules: false,
	}
}

func NamespaceOperation(operation adminv1.DomainOperation) enumsspb.NamespaceOperation {
	return enumsspb.NamespaceOperation(operation)
}

func SyncShardStatus(status *adminv1.SyncShardStatus) *repication.SyncShardStatus {
	if status == nil {
		return nil
	}

	return &repication.SyncShardStatus{
		StatusTime: Timestamp(status.GetTimestamp()),
	}
}

func Timestamp(timestamp *types.Timestamp) *timestamppb.Timestamp {
	if timestamp == nil {
		return nil
	}

	return &timestamppb.Timestamp{
		Seconds: timestamp.GetSeconds(),
		Nanos:   timestamp.GetNanos(),
	}
}

func DescribeClusterResponse(resp *adminv1.DescribeClusterResponse) *adminservice.DescribeClusterResponse {
	if resp == nil {
		return nil
	}

	return &adminservice.DescribeClusterResponse{
		SupportedClients:         nil,
		ServerVersion:            "",
		MembershipInfo:           nil,
		ClusterId:                "cadence-uuid",
		ClusterName:              "cadence-2",
		HistoryShardCount:        4,
		PersistenceStore:         "",
		VisibilityStore:          "",
		VersionInfo:              nil,
		FailoverVersionIncrement: 10,
		InitialFailoverVersion:   2,
		IsGlobalNamespaceEnabled: true,
		Tags:                     nil,
	}
}

func GetNamespaceReplicationMessagesResponse(resp *adminv1.GetDomainReplicationMessagesResponse) *adminservice.GetNamespaceReplicationMessagesResponse {
	if resp == nil || resp.GetMessages() == nil {
		return nil
	}

	messages := &repication.ReplicationMessages{
		ReplicationTasks:       ReplicationTasks(resp.GetMessages().GetReplicationTasks()),
		LastRetrievedMessageId: resp.GetMessages().GetLastRetrievedMessageId(),
		HasMore:                resp.GetMessages().GetHasMore(),
		SyncShardStatus:        SyncShardStatus(resp.GetMessages().GetSyncShardStatus()),
	}

	return &adminservice.GetNamespaceReplicationMessagesResponse{
		Messages: messages,
	}
}

func GetNamespaceReplicationMessagesRequest(request *adminv1.GetDomainReplicationMessagesRequest) *adminservice.GetNamespaceReplicationMessagesRequest {
	if request == nil {
		return nil
	}

	return &adminservice.GetNamespaceReplicationMessagesRequest{
		ClusterName:            request.GetClusterName(),
		LastRetrievedMessageId: Int64Value(request.GetLastRetrievedMessageId()),
		LastProcessedMessageId: Int64Value(request.GetLastProcessedMessageId()),
	}

}

func Int64Value(id *types.Int64Value) int64 {
	if id == nil {
		return 0
	}

	return id.GetValue()
}
