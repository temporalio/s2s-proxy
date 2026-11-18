package proxy

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/nexus-rpc/sdk-go/nexus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	proxycodec "github.com/temporalio/temporal-proxy/pkg/codec"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	failurepb "go.temporal.io/api/failure/v1"
	historypb "go.temporal.io/api/history/v1"
	nexuspb "go.temporal.io/api/nexus/v1"
	"go.temporal.io/api/operatorservice/v1"
	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdkactivity "go.temporal.io/sdk/activity"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/tests/testcore"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	proxymetrics "github.com/temporalio/s2s-proxy/metrics"
)

const (
	comprehensiveWorkflowType        = "replication-encryption-comprehensive-workflow"
	comprehensiveReceiverType        = "replication-encryption-signal-receiver"
	comprehensiveHeaderWorkflowType  = "replication-encryption-header-workflow"
	comprehensiveChildType           = "replication-encryption-child"
	comprehensiveActivityType        = "replication-encryption-activity"
	comprehensiveHeartbeatType       = "replication-encryption-heartbeat-activity"
	comprehensiveLocalActivityType   = "replication-encryption-local-activity"
	comprehensiveFailingActivityType = "replication-encryption-failing-activity"
	comprehensiveFinishSignal        = "replication-encryption-finish"
	comprehensiveExternalSignal      = "replication-encryption-external"
	comprehensiveUpdateName          = "replication-encryption-update"
	comprehensiveReadyQuery          = "replication-encryption-ready"
	comprehensiveNexusService        = "replication-encryption-nexus-service"
	comprehensiveNexusOperation      = "replication-encryption-nexus-operation"

	comprehensiveWorkflowSecret       = "canary-workflow-input-5e432f"
	comprehensiveMemoSecret           = "canary-workflow-memo-d945c3"
	comprehensiveActivityInput        = "canary-activity-input-1a296d"
	comprehensiveActivityResult       = "canary-activity-result-812ee4"
	comprehensiveHeartbeatInput       = "canary-heartbeat-input-d82e08"
	comprehensiveHeartbeatDetail      = "canary-heartbeat-detail-0f521a"
	comprehensiveHeartbeatResult      = "canary-heartbeat-result-32f662"
	comprehensiveLocalActivityInput   = "canary-local-activity-input-b43fd1"
	comprehensiveLocalActivityResult  = "canary-local-activity-result-1e6357"
	comprehensiveSideEffectResult     = "canary-side-effect-result-bf3920"
	comprehensiveFailureInput         = "canary-failure-input-72fe11"
	comprehensiveFailureDetail        = "canary-failure-detail-4d50ac"
	comprehensiveChildInput           = "canary-child-input-fb7601"
	comprehensiveChildResult          = "canary-child-result-32c004"
	comprehensiveNexusInput           = "canary-nexus-input-8be410"
	comprehensiveNexusResult          = "canary-nexus-result-d3fb2a"
	comprehensiveFinishSignalValue    = "canary-signal-input-69c9f7"
	comprehensiveExternalSignalValue  = "canary-external-signal-8d447a"
	comprehensiveUpdateInput          = "canary-update-input-8f02b4"
	comprehensiveUpdateResult         = "canary-update-result-c181e0"
	comprehensiveContinueInput        = "canary-continue-input-07b116"
	comprehensiveContinueResult       = "canary-continue-result-bc4e92"
	comprehensiveHeaderValue          = "canary-header-value-c4346c"
	comprehensiveHeaderWorkflowResult = "canary-header-result-c51cc4"
)

var (
	comprehensiveHeartbeatRecorded chan struct{}
	comprehensiveHeartbeatRelease  chan struct{}
)

type comprehensiveWorkflowInput struct {
	Secret        string
	NexusEndpoint string
	ReceiverID    string
	Generation    int
}

func comprehensiveEncryptionWorkflow(ctx workflow.Context, input comprehensiveWorkflowInput) (string, error) {
	if input.Generation == 1 {
		if input.Secret != comprehensiveContinueInput {
			return "", fmt.Errorf("continued workflow input = %q", input.Secret)
		}
		return comprehensiveContinueResult, nil
	}

	phase := "running-on-cluster-a"
	if err := workflow.SetQueryHandler(ctx, comprehensiveReadyQuery, func() (string, error) { return phase, nil }); err != nil {
		return "", err
	}
	var updateResult string
	if err := workflow.SetUpdateHandler(ctx, comprehensiveUpdateName, func(_ workflow.Context, value string) (string, error) {
		if value != comprehensiveUpdateInput {
			return "", fmt.Errorf("update input = %q", value)
		}
		updateResult = comprehensiveUpdateResult
		return updateResult, nil
	}); err != nil {
		return "", err
	}

	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout:   time.Minute,
		HeartbeatTimeout:      10 * time.Second,
		DisableEagerExecution: true,
		RetryPolicy:           &temporal.RetryPolicy{MaximumAttempts: 1},
	})
	var activityResult string
	if err := workflow.ExecuteActivity(activityCtx, comprehensiveActivityType, comprehensiveActivityInput).Get(ctx, &activityResult); err != nil {
		return "", err
	}
	var heartbeatResult string
	if err := workflow.ExecuteActivity(activityCtx, comprehensiveHeartbeatType, comprehensiveHeartbeatInput).Get(ctx, &heartbeatResult); err != nil {
		return "", err
	}
	localCtx := workflow.WithLocalActivityOptions(ctx, workflow.LocalActivityOptions{ScheduleToCloseTimeout: time.Minute})
	var localActivityResult string
	if err := workflow.ExecuteLocalActivity(localCtx, comprehensiveLocalActivity, comprehensiveLocalActivityInput).Get(ctx, &localActivityResult); err != nil {
		return "", err
	}
	var sideEffectResult string
	if err := workflow.SideEffect(ctx, func(workflow.Context) interface{} { return comprehensiveSideEffectResult }).Get(&sideEffectResult); err != nil {
		return "", err
	}
	if err := workflow.ExecuteActivity(activityCtx, comprehensiveFailingActivityType, comprehensiveFailureInput).Get(ctx, nil); err == nil {
		return "", fmt.Errorf("failing activity unexpectedly succeeded")
	}
	if err := workflow.Sleep(ctx, 100*time.Millisecond); err != nil {
		return "", err
	}
	var childResult string
	if err := workflow.ExecuteChildWorkflow(ctx, comprehensiveChildType, comprehensiveChildInput).Get(ctx, &childResult); err != nil {
		return "", err
	}
	if err := workflow.SignalExternalWorkflow(ctx, input.ReceiverID, "", comprehensiveExternalSignal, comprehensiveExternalSignalValue).Get(ctx, nil); err != nil {
		return "", err
	}
	var nexusResult string
	if err := workflow.NewNexusClient(input.NexusEndpoint, comprehensiveNexusService).
		ExecuteOperation(ctx, comprehensiveNexusOperation, comprehensiveNexusInput, workflow.NexusOperationOptions{}).
		Get(ctx, &nexusResult); err != nil {
		return "", err
	}

	phase = "waiting-for-failover"
	var signalValue string
	workflow.GetSignalChannel(ctx, comprehensiveFinishSignal).Receive(ctx, &signalValue)
	if signalValue != comprehensiveFinishSignalValue || updateResult != comprehensiveUpdateResult {
		return "", fmt.Errorf("post-failover values signal=%q update=%q", signalValue, updateResult)
	}
	_ = strings.Join([]string{input.Secret, activityResult, heartbeatResult, localActivityResult, sideEffectResult, childResult, nexusResult}, "|")
	return "", workflow.NewContinueAsNewError(ctx, comprehensiveWorkflowType, comprehensiveWorkflowInput{
		Secret: comprehensiveContinueInput, NexusEndpoint: input.NexusEndpoint, ReceiverID: input.ReceiverID, Generation: 1,
	})
}

func comprehensiveSignalReceiver(ctx workflow.Context) (string, error) {
	var value string
	workflow.GetSignalChannel(ctx, comprehensiveExternalSignal).Receive(ctx, &value)
	if value != comprehensiveExternalSignalValue {
		return "", fmt.Errorf("external signal = %q", value)
	}
	return value, nil
}

func comprehensiveHeaderWorkflow(workflow.Context) (string, error) {
	return comprehensiveHeaderWorkflowResult, nil
}

func comprehensiveChildWorkflow(_ workflow.Context, value string) (string, error) {
	if value != comprehensiveChildInput {
		return "", fmt.Errorf("child input = %q", value)
	}
	return comprehensiveChildResult, nil
}

func comprehensiveActivity(_ context.Context, value string) (string, error) {
	if value != comprehensiveActivityInput {
		return "", fmt.Errorf("activity input = %q", value)
	}
	return comprehensiveActivityResult, nil
}

func comprehensiveHeartbeatActivity(ctx context.Context, value string) (string, error) {
	if value != comprehensiveHeartbeatInput {
		return "", fmt.Errorf("heartbeat activity input = %q", value)
	}
	sdkactivity.RecordHeartbeat(ctx, comprehensiveHeartbeatDetail)
	select {
	case comprehensiveHeartbeatRecorded <- struct{}{}:
	default:
	}
	select {
	case <-comprehensiveHeartbeatRelease:
		return comprehensiveHeartbeatResult, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func comprehensiveLocalActivity(_ context.Context, value string) (string, error) {
	if value != comprehensiveLocalActivityInput {
		return "", fmt.Errorf("local activity input = %q", value)
	}
	return comprehensiveLocalActivityResult, nil
}

func comprehensiveFailingActivity(_ context.Context, value string) error {
	if value != comprehensiveFailureInput {
		return fmt.Errorf("failing activity input = %q", value)
	}
	return temporal.NewNonRetryableApplicationError("expected test failure", "ExpectedFailure", nil, comprehensiveFailureDetail)
}

func (s *ReplicationTestSuite) TestEncryptedReplicationComprehensiveWorkflow() {
	if !s.encryptionEnabled {
		s.T().Skip("requires the encrypted replication topology")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	taskQueue := "encrypted-replication-comprehensive"
	workflowID := "encrypted-replication-comprehensive-" + fmt.Sprint(time.Now().UnixNano())
	clientA := s.newSDKClient(s.clusterA.Host().FrontendGRPCAddress())
	defer clientA.Close()

	endpointName := testcore.RandomizedNexusEndpoint(s.T().Name())
	endpoint, err := s.clusterA.OperatorClient().CreateNexusEndpoint(ctx, &operatorservice.CreateNexusEndpointRequest{
		Spec: &nexuspb.EndpointSpec{
			Name: endpointName,
			Target: &nexuspb.EndpointTarget{Variant: &nexuspb.EndpointTarget_Worker_{
				Worker: &nexuspb.EndpointTarget_Worker{Namespace: s.namespace, TaskQueue: taskQueue},
			}},
		},
	})
	s.Require().NoError(err)
	s.T().Cleanup(func() {
		_, _ = s.clusterA.OperatorClient().DeleteNexusEndpoint(testcore.NewContext(), &operatorservice.DeleteNexusEndpointRequest{
			Id: endpoint.GetEndpoint().GetId(), Version: endpoint.GetEndpoint().GetVersion(),
		})
	})

	comprehensiveHeartbeatRecorded = make(chan struct{}, 1)
	comprehensiveHeartbeatRelease = make(chan struct{})
	workerA := s.newComprehensiveWorker(clientA, taskQueue, "cluster-a-comprehensive-worker")
	s.Require().NoError(workerA.Start())
	workerAStopped := false
	defer func() {
		if !workerAStopped {
			workerA.Stop()
		}
	}()

	headerPayload, err := converter.GetDefaultDataConverter().ToPayload(comprehensiveHeaderValue)
	s.Require().NoError(err)
	headerWorkflowID := workflowID + "-header"
	headerStart, err := s.clusterA.FrontendClient().StartWorkflowExecution(ctx, &workflowservice.StartWorkflowExecutionRequest{
		Namespace:           s.namespace,
		WorkflowId:          headerWorkflowID,
		WorkflowType:        &commonpb.WorkflowType{Name: comprehensiveHeaderWorkflowType},
		TaskQueue:           &taskqueuepb.TaskQueue{Name: taskQueue, Kind: enumspb.TASK_QUEUE_KIND_NORMAL},
		RequestId:           common.GenerateRandomString(10),
		Identity:            "replication-encryption-test",
		Header:              &commonpb.Header{Fields: map[string]*commonpb.Payload{"sensitive": headerPayload}},
		WorkflowRunTimeout:  durationpb.New(time.Minute),
		WorkflowTaskTimeout: durationpb.New(10 * time.Second),
	})
	s.Require().NoError(err)
	headerRun := clientA.GetWorkflow(ctx, headerWorkflowID, headerStart.GetRunId())

	receiverRun, err := clientA.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
		ID: workflowID + "-receiver", TaskQueue: taskQueue,
	}, comprehensiveReceiverType)
	s.Require().NoError(err)
	run, err := clientA.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
		ID: workflowID, TaskQueue: taskQueue,
		Memo: map[string]interface{}{"sensitive": comprehensiveMemoSecret},
	}, comprehensiveWorkflowType, comprehensiveWorkflowInput{
		Secret: comprehensiveWorkflowSecret, NexusEndpoint: endpointName, ReceiverID: receiverRun.GetID(),
	})
	s.Require().NoError(err)
	initialRunID := run.GetRunID()

	select {
	case <-comprehensiveHeartbeatRecorded:
	case <-ctx.Done():
		s.T().Fatal(ctx.Err())
	}
	s.Eventually(func() bool {
		state, stateErr := s.clusterB.AdminClient().DescribeMutableState(ctx, &adminservice.DescribeMutableStateRequest{
			Namespace: s.namespace,
			Execution: &commonpb.WorkflowExecution{WorkflowId: workflowID, RunId: initialRunID},
		})
		if stateErr != nil {
			return false
		}
		for _, activityInfo := range state.GetDatabaseMutableState().GetActivityInfos() {
			if activityInfo.GetActivityType().GetName() != comprehensiveHeartbeatType {
				continue
			}
			payloads := activityInfo.GetLastHeartbeatDetails().GetPayloads()
			if len(payloads) == 0 {
				return false
			}
			for _, payload := range payloads {
				if string(payload.GetMetadata()[proxycodec.MetadataEncoding]) != proxycodec.EncryptionEncoding ||
					bytes.Contains(payload.GetData(), []byte(comprehensiveHeartbeatDetail)) {
					return false
				}
			}
			return true
		}
		return false
	}, 20*time.Second, 100*time.Millisecond, "Cluster B did not persist the sealed heartbeat detail")
	close(comprehensiveHeartbeatRelease)

	s.Eventually(func() bool {
		value, queryErr := clientA.QueryWorkflow(ctx, workflowID, initialRunID, comprehensiveReadyQuery)
		if queryErr != nil {
			return false
		}
		var phase string
		return value.Get(&phase) == nil && phase == "waiting-for-failover"
	}, 30*time.Second, 100*time.Millisecond, "workflow did not reach the failover barrier")
	updateHandle, err := clientA.UpdateWorkflow(ctx, sdkclient.UpdateWorkflowOptions{
		WorkflowID: workflowID, RunID: initialRunID, UpdateName: comprehensiveUpdateName,
		Args: []interface{}{comprehensiveUpdateInput}, WaitForStage: sdkclient.WorkflowUpdateStageCompleted,
	})
	s.Require().NoError(err)
	var updateResult string
	s.Require().NoError(updateHandle.Get(ctx, &updateResult))
	s.Equal(comprehensiveUpdateResult, updateResult)
	var receiverResult string
	s.Require().NoError(receiverRun.Get(ctx, &receiverResult))
	s.Equal(comprehensiveExternalSignalValue, receiverResult)
	var headerResult string
	s.Require().NoError(headerRun.Get(ctx, &headerResult))
	s.Equal(comprehensiveHeaderWorkflowResult, headerResult)

	workerA.Stop()
	workerAStopped = true
	s.waitForClusterSynced()
	preFailoverHistory := s.getHistory(ctx, s.clusterB.FrontendClient(), workflowID, initialRunID)
	s.assertComprehensiveHistorySealed(preFailoverHistory)

	proxyClient := s.newSDKClient(s.proxyAOutbound)
	defer proxyClient.Close()
	decryptsBefore := testutil.ToFloat64(proxymetrics.EncryptionDEKOps.WithLabelValues("decrypt", "success"))
	s.failoverNamespace(ctx, s.namespace, s.clusterB.ClusterName())
	workerB := s.newComprehensiveWorker(proxyClient, taskQueue, "cluster-b-comprehensive-worker")
	s.Require().NoError(workerB.Start())
	defer workerB.Stop()
	s.Require().NoError(proxyClient.SignalWorkflow(ctx, workflowID, initialRunID, comprehensiveFinishSignal, comprehensiveFinishSignalValue))
	var result string
	s.Require().NoError(proxyClient.GetWorkflow(ctx, workflowID, initialRunID).Get(ctx, &result))
	s.Equal(comprehensiveContinueResult, result)
	decryptsAfter := testutil.ToFloat64(proxymetrics.EncryptionDEKOps.WithLabelValues("decrypt", "success"))
	s.Greater(decryptsAfter, decryptsBefore, "post-failover worker did not decrypt replicated payloads")

	initialHistory := s.getHistory(ctx, s.clusterB.FrontendClient(), workflowID, initialRunID)
	childExecution := comprehensiveChildExecution(s.T(), initialHistory.GetEvents())
	continuedExecution := comprehensiveContinuedExecution(s.T(), workflowID, initialHistory.GetEvents())
	histories := map[string]*historypb.History{
		"initial":   initialHistory,
		"continued": s.getHistory(ctx, s.clusterB.FrontendClient(), continuedExecution.GetWorkflowId(), continuedExecution.GetRunId()),
		"child":     s.getHistory(ctx, s.clusterB.FrontendClient(), childExecution.GetWorkflowId(), childExecution.GetRunId()),
		"receiver":  s.getHistory(ctx, s.clusterB.FrontendClient(), receiverRun.GetID(), receiverRun.GetRunID()),
		"header":    s.getHistory(ctx, s.clusterB.FrontendClient(), headerWorkflowID, headerStart.GetRunId()),
	}
	for label, history := range histories {
		s.assertComprehensiveHistorySealed(history)
		s.assertNoComprehensivePlaintext(label+" history", history, comprehensiveAllCanaries())
	}

	executions := map[string]*commonpb.WorkflowExecution{
		"initial":   {WorkflowId: workflowID, RunId: initialRunID},
		"continued": continuedExecution,
		"child":     childExecution,
		"receiver":  {WorkflowId: receiverRun.GetID(), RunId: receiverRun.GetRunID()},
		"header":    {WorkflowId: headerWorkflowID, RunId: headerStart.GetRunId()},
	}
	for label, execution := range executions {
		state, stateErr := s.clusterB.AdminClient().DescribeMutableState(ctx, &adminservice.DescribeMutableStateRequest{
			Namespace: s.namespace, Execution: execution,
		})
		s.Require().NoError(stateErr)
		if label == "initial" {
			hasTransitionHistory := len(state.GetDatabaseMutableState().GetExecutionInfo().GetTransitionHistory()) > 0
			s.Equal(s.transitionHistoryEnabled, hasTransitionHistory, "test did not exercise the requested replication mode")
		}
		s.assertNoComprehensivePlaintext(label+" mutable state", state, comprehensiveAllCanaries())
	}

	s.waitForClusterSynced()
	s.T().Logf("comprehensive encrypted replication passed: transition_history=%t decrypt_success_delta=%.0f", s.transitionHistoryEnabled, decryptsAfter-decryptsBefore)
}

func (s *ReplicationTestSuite) newComprehensiveWorker(client sdkclient.Client, taskQueue, identity string) worker.Worker {
	w := worker.New(client, taskQueue, worker.Options{Identity: identity})
	w.RegisterWorkflowWithOptions(comprehensiveEncryptionWorkflow, workflow.RegisterOptions{Name: comprehensiveWorkflowType})
	w.RegisterWorkflowWithOptions(comprehensiveSignalReceiver, workflow.RegisterOptions{Name: comprehensiveReceiverType})
	w.RegisterWorkflowWithOptions(comprehensiveHeaderWorkflow, workflow.RegisterOptions{Name: comprehensiveHeaderWorkflowType})
	w.RegisterWorkflowWithOptions(comprehensiveChildWorkflow, workflow.RegisterOptions{Name: comprehensiveChildType})
	w.RegisterActivityWithOptions(comprehensiveActivity, sdkactivity.RegisterOptions{Name: comprehensiveActivityType})
	w.RegisterActivityWithOptions(comprehensiveHeartbeatActivity, sdkactivity.RegisterOptions{Name: comprehensiveHeartbeatType})
	w.RegisterActivityWithOptions(comprehensiveLocalActivity, sdkactivity.RegisterOptions{Name: comprehensiveLocalActivityType})
	w.RegisterActivityWithOptions(comprehensiveFailingActivity, sdkactivity.RegisterOptions{Name: comprehensiveFailingActivityType})
	nexusService := nexus.NewService(comprehensiveNexusService)
	nexusService.MustRegister(nexus.NewSyncOperation(comprehensiveNexusOperation, func(_ context.Context, input string, _ nexus.StartOperationOptions) (string, error) {
		if input != comprehensiveNexusInput {
			return "", fmt.Errorf("nexus input = %q", input)
		}
		return comprehensiveNexusResult, nil
	}))
	w.RegisterNexusService(nexusService)
	return w
}

func comprehensivePayloads(events []*historypb.HistoryEvent) []*commonpb.Payload {
	var payloads []*commonpb.Payload
	appendPayloads := func(values *commonpb.Payloads) { payloads = append(payloads, values.GetPayloads()...) }
	appendPayload := func(value *commonpb.Payload) {
		if value != nil {
			payloads = append(payloads, value)
		}
	}
	for _, event := range events {
		if attributes := event.GetWorkflowExecutionStartedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
			for _, payload := range attributes.GetMemo().GetFields() {
				appendPayload(payload)
			}
			for _, payload := range attributes.GetHeader().GetFields() {
				appendPayload(payload)
			}
		}
		if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
		}
		if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetResult())
		}
		if attributes := event.GetActivityTaskFailedEventAttributes(); attributes != nil {
			comprehensiveAppendFailurePayloads(attributes.GetFailure(), appendPayloads)
		}
		if attributes := event.GetMarkerRecordedEventAttributes(); attributes != nil {
			for _, details := range attributes.GetDetails() {
				appendPayloads(details)
			}
			comprehensiveAppendFailurePayloads(attributes.GetFailure(), appendPayloads)
		}
		if attributes := event.GetWorkflowExecutionSignaledEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
		}
		if attributes := event.GetSignalExternalWorkflowExecutionInitiatedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
		}
		if attributes := event.GetStartChildWorkflowExecutionInitiatedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
		}
		if attributes := event.GetChildWorkflowExecutionCompletedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetResult())
		}
		if attributes := event.GetNexusOperationScheduledEventAttributes(); attributes != nil {
			appendPayload(attributes.GetInput())
		}
		if attributes := event.GetNexusOperationCompletedEventAttributes(); attributes != nil {
			appendPayload(attributes.GetResult())
		}
		if attributes := event.GetWorkflowExecutionUpdateAcceptedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetAcceptedRequest().GetInput().GetArgs())
		}
		if attributes := event.GetWorkflowExecutionUpdateCompletedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetOutcome().GetSuccess())
			comprehensiveAppendFailurePayloads(attributes.GetOutcome().GetFailure(), appendPayloads)
		}
		if attributes := event.GetWorkflowExecutionContinuedAsNewEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
		}
		if attributes := event.GetWorkflowExecutionCompletedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetResult())
		}
	}
	return payloads
}

func comprehensiveAppendFailurePayloads(failure *failurepb.Failure, appendPayloads func(*commonpb.Payloads)) {
	if failure == nil {
		return
	}
	if info := failure.GetApplicationFailureInfo(); info != nil {
		appendPayloads(info.GetDetails())
	}
	comprehensiveAppendFailurePayloads(failure.GetCause(), appendPayloads)
}

func (s *ReplicationTestSuite) assertComprehensiveHistorySealed(history *historypb.History) {
	payloads := comprehensivePayloads(history.GetEvents())
	s.NotEmpty(payloads, "history contained no application payloads")
	for _, payload := range payloads {
		encoding := string(payload.GetMetadata()[proxycodec.MetadataEncoding])
		s.Equal(proxycodec.EncryptionEncoding, encoding, "unexpected plaintext payload data=%q", payload.GetData())
	}
}

func (s *ReplicationTestSuite) assertNoComprehensivePlaintext(label string, message proto.Message, canaries []string) {
	wire, err := proto.Marshal(message)
	s.Require().NoError(err)
	for _, canary := range canaries {
		s.NotContains(string(wire), canary, "%s contains plaintext canary", label)
	}
}

func comprehensiveChildExecution(t interface{ Fatal(args ...interface{}) }, events []*historypb.HistoryEvent) *commonpb.WorkflowExecution {
	for _, event := range events {
		if attributes := event.GetChildWorkflowExecutionStartedEventAttributes(); attributes != nil {
			return attributes.GetWorkflowExecution()
		}
	}
	t.Fatal("history is missing child workflow execution")
	return nil
}

func comprehensiveContinuedExecution(t interface{ Fatal(args ...interface{}) }, workflowID string, events []*historypb.HistoryEvent) *commonpb.WorkflowExecution {
	for _, event := range events {
		if attributes := event.GetWorkflowExecutionContinuedAsNewEventAttributes(); attributes != nil {
			return &commonpb.WorkflowExecution{WorkflowId: workflowID, RunId: attributes.GetNewExecutionRunId()}
		}
	}
	t.Fatal("history is missing continued-as-new execution")
	return nil
}

func comprehensiveAllCanaries() []string {
	return []string{
		comprehensiveWorkflowSecret, comprehensiveMemoSecret,
		comprehensiveActivityInput, comprehensiveActivityResult,
		comprehensiveHeartbeatInput, comprehensiveHeartbeatDetail, comprehensiveHeartbeatResult,
		comprehensiveLocalActivityInput, comprehensiveLocalActivityResult, comprehensiveSideEffectResult,
		comprehensiveFailureInput, comprehensiveFailureDetail,
		comprehensiveChildInput, comprehensiveChildResult,
		comprehensiveNexusInput,
		comprehensiveFinishSignalValue, comprehensiveExternalSignalValue,
		comprehensiveUpdateInput, comprehensiveUpdateResult,
		comprehensiveContinueInput, comprehensiveContinueResult,
		comprehensiveHeaderValue, comprehensiveHeaderWorkflowResult,
		comprehensiveNexusResult,
	}
}
