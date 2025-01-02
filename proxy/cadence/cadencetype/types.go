package cadencetype

import (
	"fmt"
	"github.com/gogo/protobuf/types"
	cadenceadmin "github.com/uber/cadence-idl/go/proto/admin/v1"
	cadence "github.com/uber/cadence-idl/go/proto/api/v1"
	temporal "go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/namespace/v1"
	"go.temporal.io/api/replication/v1"
	"go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflow/v1"
	enumsspb "go.temporal.io/server/api/enums/v1"
	temporalreplication "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func WorkflowExecution(e *temporal.WorkflowExecution) *cadence.WorkflowExecution {
	return &cadence.WorkflowExecution{
		WorkflowId: e.GetWorkflowId(),
		RunId:      e.GetRunId(),
	}
}

func WorkflowType(t *temporal.WorkflowType) *cadence.WorkflowType {
	return &cadence.WorkflowType{
		Name: t.GetName(),
	}
}

func Int64ValuePtr(i int64) *types.Int64Value {
	return &types.Int64Value{
		Value: i,
	}
}

func TimeoutType(timeoutType enums.TimeoutType) cadence.TimeoutType {
	return cadence.TimeoutType(timeoutType)
}

func ParentExecutionInfo(parentExecution *temporal.WorkflowExecution) *cadence.ParentExecutionInfo {
	if parentExecution == nil {
		return nil
	}

	return &cadence.ParentExecutionInfo{
		//DomainId:          "",
		//DomainName:        "",
		WorkflowExecution: &cadence.WorkflowExecution{
			WorkflowId: parentExecution.GetWorkflowId(),
			RunId:      parentExecution.GetRunId(),
		},
		//InitiatedId:       0,
	}
}

func Initiator(initiator enums.ContinueAsNewInitiator) cadence.ContinueAsNewInitiator {
	switch initiator {
	case enums.CONTINUE_AS_NEW_INITIATOR_WORKFLOW:
		return cadence.ContinueAsNewInitiator_CONTINUE_AS_NEW_INITIATOR_DECIDER
	case enums.CONTINUE_AS_NEW_INITIATOR_RETRY:
		return cadence.ContinueAsNewInitiator_CONTINUE_AS_NEW_INITIATOR_RETRY_POLICY
	case enums.CONTINUE_AS_NEW_INITIATOR_CRON_SCHEDULE:
		return cadence.ContinueAsNewInitiator_CONTINUE_AS_NEW_INITIATOR_CRON_SCHEDULE
	default:
		return cadence.ContinueAsNewInitiator_CONTINUE_AS_NEW_INITIATOR_INVALID
	}
}

func ReseetPoints(points *workflow.ResetPoints) *cadence.ResetPoints {
	return nil
}

func Header(header *temporal.Header) *cadence.Header {
	return nil
}

func SearchAttributes(attributes *temporal.SearchAttributes) *cadence.SearchAttributes {
	return nil
}

func Memo(memo *temporal.Memo) *cadence.Memo {
	return nil
}

func RetryPolicy(policy *temporal.RetryPolicy) *cadence.RetryPolicy {
	if policy == nil {
		return nil
	}

	return &cadence.RetryPolicy{
		InitialInterval:          Duration(policy.GetInitialInterval()),
		BackoffCoefficient:       policy.GetBackoffCoefficient(),
		MaximumInterval:          Duration(policy.GetMaximumInterval()),
		MaximumAttempts:          policy.GetMaximumAttempts(),
		NonRetryableErrorReasons: policy.GetNonRetryableErrorTypes(),
		//ExpirationInterval:       nil,
	}
}

func Payloads(payloads *temporal.Payloads) *cadence.Payload {
	if payloads == nil || len(payloads.GetPayloads()) == 0 {
		return nil
	}

	if len(payloads.GetPayloads()) > 1 {
		panic(fmt.Sprintf("more than one payload is not supported: %v", payloads.GetPayloads()))
	}

	return Payload(payloads.GetPayloads()[0])
}

func Payload(payload *temporal.Payload) *cadence.Payload {
	if payload == nil {
		return nil
	}

	return &cadence.Payload{
		Data: payload.GetData(),
	}
}

func Failure(failure *failure.Failure) *cadence.Failure {
	if failure == nil {
		return nil
	}

	return &cadence.Failure{
		Reason: failure.GetMessage(),
	}
}

func Duration(d *durationpb.Duration) *types.Duration {
	if d == nil {
		return nil
	}

	return &types.Duration{
		Seconds: d.GetSeconds(),
		Nanos:   d.GetNanos(),
	}
}

func TaskList(tq *taskqueue.TaskQueue) *cadence.TaskList {
	if tq == nil {
		return nil
	}

	return &cadence.TaskList{
		Name: tq.GetName(),
		Kind: TaskListKind(tq.GetKind()),
	}
}

func TaskListKind(kind enums.TaskQueueKind) cadence.TaskListKind {
	switch kind {
	case enums.TASK_QUEUE_KIND_NORMAL:
		return cadence.TaskListKind_TASK_LIST_KIND_NORMAL
	case enums.TASK_QUEUE_KIND_STICKY:
		return cadence.TaskListKind_TASK_LIST_KIND_STICKY
	case enums.TASK_QUEUE_KIND_UNSPECIFIED:
		return cadence.TaskListKind_TASK_LIST_KIND_INVALID
	default:
		return cadence.TaskListKind_TASK_LIST_KIND_INVALID
	}
}

func Timestamp(t *timestamppb.Timestamp) *types.Timestamp {
	if t == nil {
		return nil
	}

	return &types.Timestamp{
		Seconds: t.GetSeconds(),
		Nanos:   t.GetNanos(),
	}
}

func Error(err error, logger log.Logger) error {
	if err != nil {
		logger.Error("Operation error", tag.Error(err))
	}
	return err
}

func ShardMessages(messages map[int32]*temporalreplication.ReplicationMessages) map[int32]*cadenceadmin.ReplicationMessages {
	if messages == nil {
		return nil
	}

	result := make(map[int32]*cadenceadmin.ReplicationMessages, len(messages))
	for k, v := range messages {
		result[k-1] = ReplicationMessages(v)
	}
	return result
}

func Domain(
	info *namespace.NamespaceInfo,
	config *namespace.NamespaceConfig,
	replicationConfig *replication.NamespaceReplicationConfig,
	failoverVersion int64,
) *cadence.Domain {
	if info == nil {
		return nil
	}

	return &cadence.Domain{
		Id:                               info.GetId(),
		Name:                             info.GetName(),
		Status:                           DomainStatus(info.GetState()),
		Description:                      info.GetDescription(),
		OwnerEmail:                       info.GetOwnerEmail(),
		Data:                             info.GetData(),
		WorkflowExecutionRetentionPeriod: Duration(config.GetWorkflowExecutionRetentionTtl()),
		BadBinaries:                      BadBinaries(config.GetBadBinaries()),
		HistoryArchivalStatus:            ArchivalStatus(config.GetHistoryArchivalState()),
		HistoryArchivalUri:               config.GetVisibilityArchivalUri(),
		VisibilityArchivalStatus:         ArchivalStatus(config.GetVisibilityArchivalState()),
		VisibilityArchivalUri:            config.GetVisibilityArchivalUri(),
		ActiveClusterName:                replicationConfig.GetActiveClusterName(),
		Clusters:                         Clusters(replicationConfig.GetClusters()),
		FailoverVersion:                  failoverVersion,
		IsGlobalDomain:                   true,
		//FailoverInfo:                     nil,
		//IsolationGroups:                  nil,
		//AsyncWorkflowConfig:              nil,
	}
}

func Clusters(clusters []*replication.ClusterReplicationConfig) []*cadence.ClusterReplicationConfiguration {
	if clusters == nil {
		return nil
	}

	result := make([]*cadence.ClusterReplicationConfiguration, 0, len(clusters))
	for _, c := range clusters {
		result = append(result, &cadence.ClusterReplicationConfiguration{
			ClusterName: c.GetClusterName(),
		})
	}
	return result
}

func ArchivalStatus(state enums.ArchivalState) cadence.ArchivalStatus {
	return cadence.ArchivalStatus(state)
}

func BadBinaries(badBinaries *namespace.BadBinaries) *cadence.BadBinaries {
	if badBinaries == nil {
		return nil
	}

	binaries := make(map[string]*cadence.BadBinaryInfo, len(badBinaries.GetBinaries()))
	for k, v := range badBinaries.GetBinaries() {
		binaries[k] = &cadence.BadBinaryInfo{
			Reason:      v.GetReason(),
			Operator:    v.GetOperator(),
			CreatedTime: Timestamp(v.GetCreateTime()),
		}
	}

	return &cadence.BadBinaries{
		Binaries: binaries,
	}
}

func DomainStatus(state enums.NamespaceState) cadence.DomainStatus {
	return cadence.DomainStatus(state)
}

func DomainOperation(operation enumsspb.NamespaceOperation) cadenceadmin.DomainOperation {
	return cadenceadmin.DomainOperation(operation)
}

func Int64Value(id int64) *types.Int64Value {
	return &types.Int64Value{
		Value: id,
	}
}
