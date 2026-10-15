package proxy

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nexus-rpc/sdk-go/nexus"
	"github.com/stretchr/testify/require"
	proxycodec "github.com/temporalio/temporal-proxy/pkg/codec"
	commonpb "go.temporal.io/api/common/v1"
	historypb "go.temporal.io/api/history/v1"
	nexuspb "go.temporal.io/api/nexus/v1"
	operatorservice "go.temporal.io/api/operatorservice/v1"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	sdkactivity "go.temporal.io/sdk/activity"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/tests/testcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
)

const (
	primitiveWorkflowType       = "proxy-encryption-primitives-workflow"
	primitiveReceiverType       = "proxy-encryption-primitives-receiver"
	primitiveChildType          = "proxy-encryption-primitives-child"
	primitiveActivityType       = "proxy-encryption-primitives-activity"
	primitiveHeartbeatType      = "proxy-encryption-primitives-heartbeat"
	primitiveLocalActivityType  = "proxy-encryption-primitives-local-activity"
	primitiveFailingActivity    = "proxy-encryption-primitives-failing-activity"
	primitiveFinishSignal       = "proxy-encryption-primitives-finish"
	primitiveExternalSignal     = "proxy-encryption-primitives-external"
	primitiveUpdateName         = "proxy-encryption-primitives-update"
	primitiveReadyQuery         = "proxy-encryption-primitives-ready"
	primitiveNexusService       = "proxy-encryption-primitives-nexus-service"
	primitiveNexusOperation     = "proxy-encryption-primitives-nexus-operation"
	primitiveWorkflowInput      = "canary-primitive-workflow-input"
	primitiveMemo               = "canary-primitive-memo"
	primitiveActivityInput      = "canary-primitive-activity-input"
	primitiveActivityResult     = "canary-primitive-activity-result"
	primitiveHeartbeatInput     = "canary-primitive-heartbeat-input"
	primitiveHeartbeatDetail    = "canary-primitive-heartbeat-detail"
	primitiveHeartbeatResult    = "canary-primitive-heartbeat-result"
	primitiveLocalInput         = "canary-primitive-local-input"
	primitiveLocalResult        = "canary-primitive-local-result"
	primitiveSideEffectResult   = "canary-primitive-side-effect-result"
	primitiveFailureInput       = "canary-primitive-failure-input"
	primitiveFailureDetail      = "canary-primitive-failure-detail"
	primitiveChildInput         = "canary-primitive-child-input"
	primitiveChildResult        = "canary-primitive-child-result"
	primitiveNexusInput         = "canary-primitive-nexus-input"
	primitiveNexusResult        = "canary-primitive-nexus-result"
	primitiveFinishSignalValue  = "canary-primitive-finish-signal"
	primitiveExternalSignalData = "canary-primitive-external-signal"
	primitiveUpdateInput        = "canary-primitive-update-input"
	primitiveUpdateResult       = "canary-primitive-update-result"
	primitiveContinueInput      = "canary-primitive-continue-input"
	primitiveContinueResult     = "canary-primitive-continue-result"
)

var (
	primitiveHeartbeatRecorded chan struct{}
	primitiveHeartbeatRelease  chan struct{}
	primitiveCanaries          = []string{
		primitiveWorkflowInput,
		primitiveMemo,
		primitiveActivityInput,
		primitiveActivityResult,
		primitiveHeartbeatInput,
		primitiveHeartbeatDetail,
		primitiveHeartbeatResult,
		primitiveLocalInput,
		primitiveLocalResult,
		primitiveSideEffectResult,
		primitiveFailureInput,
		primitiveFailureDetail,
		primitiveChildInput,
		primitiveChildResult,
		primitiveNexusInput,
		primitiveNexusResult,
		primitiveFinishSignalValue,
		primitiveExternalSignalData,
		primitiveUpdateInput,
		primitiveUpdateResult,
		primitiveContinueInput,
		primitiveContinueResult,
	}
)

type primitiveWorkflowArgs struct {
	Secret        string
	NexusEndpoint string
	ReceiverID    string
	Generation    int
}

func primitiveEncryptionWorkflow(ctx workflow.Context, input primitiveWorkflowArgs) (string, error) {
	if input.Generation == 1 {
		if input.Secret != primitiveContinueInput {
			return "", fmt.Errorf("continued workflow received unexpected input %q", input.Secret)
		}
		return primitiveContinueResult, nil
	}
	if input.Secret != primitiveWorkflowInput {
		return "", fmt.Errorf("workflow received unexpected input %q", input.Secret)
	}

	phase := "running"
	if err := workflow.SetQueryHandler(ctx, primitiveReadyQuery, func() (string, error) { return phase, nil }); err != nil {
		return "", err
	}
	var updateResult string
	if err := workflow.SetUpdateHandler(ctx, primitiveUpdateName, func(_ workflow.Context, value string) (string, error) {
		if value != primitiveUpdateInput {
			return "", fmt.Errorf("update received unexpected input %q", value)
		}
		updateResult = primitiveUpdateResult
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
	if err := workflow.ExecuteActivity(activityCtx, primitiveActivityType, primitiveActivityInput).Get(ctx, &activityResult); err != nil {
		return "", err
	}
	if activityResult != primitiveActivityResult {
		return "", fmt.Errorf("workflow received unexpected activity result %q", activityResult)
	}

	var heartbeatResult string
	if err := workflow.ExecuteActivity(activityCtx, primitiveHeartbeatType, primitiveHeartbeatInput).Get(ctx, &heartbeatResult); err != nil {
		return "", err
	}
	if heartbeatResult != primitiveHeartbeatResult {
		return "", fmt.Errorf("workflow received unexpected heartbeat result %q", heartbeatResult)
	}

	localCtx := workflow.WithLocalActivityOptions(ctx, workflow.LocalActivityOptions{ScheduleToCloseTimeout: time.Minute})
	var localResult string
	if err := workflow.ExecuteLocalActivity(localCtx, primitiveLocalActivityType, primitiveLocalInput).Get(ctx, &localResult); err != nil {
		return "", err
	}
	if localResult != primitiveLocalResult {
		return "", fmt.Errorf("workflow received unexpected local activity result %q", localResult)
	}

	var sideEffectResult string
	if err := workflow.SideEffect(ctx, func(workflow.Context) interface{} { return primitiveSideEffectResult }).Get(&sideEffectResult); err != nil {
		return "", err
	}
	if sideEffectResult != primitiveSideEffectResult {
		return "", fmt.Errorf("workflow received unexpected side effect result %q", sideEffectResult)
	}

	if err := workflow.ExecuteActivity(activityCtx, primitiveFailingActivity, primitiveFailureInput).Get(ctx, nil); err == nil {
		return "", fmt.Errorf("failing activity unexpectedly succeeded")
	}
	if err := workflow.Sleep(ctx, 100*time.Millisecond); err != nil {
		return "", err
	}

	var childResult string
	if err := workflow.ExecuteChildWorkflow(ctx, primitiveChildType, primitiveChildInput).Get(ctx, &childResult); err != nil {
		return "", err
	}
	if childResult != primitiveChildResult {
		return "", fmt.Errorf("workflow received unexpected child result %q", childResult)
	}

	if err := workflow.SignalExternalWorkflow(
		ctx,
		input.ReceiverID,
		"",
		primitiveExternalSignal,
		primitiveExternalSignalData,
	).Get(ctx, nil); err != nil {
		return "", err
	}

	var nexusResult string
	if err := workflow.NewNexusClient(input.NexusEndpoint, primitiveNexusService).
		ExecuteOperation(ctx, primitiveNexusOperation, primitiveNexusInput, workflow.NexusOperationOptions{}).
		Get(ctx, &nexusResult); err != nil {
		return "", err
	}
	if nexusResult != primitiveNexusResult {
		return "", fmt.Errorf("workflow received unexpected Nexus result %q", nexusResult)
	}

	phase = "waiting-for-signal"
	var signalValue string
	workflow.GetSignalChannel(ctx, primitiveFinishSignal).Receive(ctx, &signalValue)
	if signalValue != primitiveFinishSignalValue || updateResult != primitiveUpdateResult {
		return "", fmt.Errorf("post-barrier values signal=%q update=%q", signalValue, updateResult)
	}

	return "", workflow.NewContinueAsNewError(ctx, primitiveWorkflowType, primitiveWorkflowArgs{
		Secret:        primitiveContinueInput,
		NexusEndpoint: input.NexusEndpoint,
		ReceiverID:    input.ReceiverID,
		Generation:    1,
	})
}

func primitiveSignalReceiver(ctx workflow.Context) (string, error) {
	var value string
	workflow.GetSignalChannel(ctx, primitiveExternalSignal).Receive(ctx, &value)
	if value != primitiveExternalSignalData {
		return "", fmt.Errorf("receiver got unexpected signal %q", value)
	}
	return value, nil
}

func primitiveChildWorkflow(_ workflow.Context, value string) (string, error) {
	if value != primitiveChildInput {
		return "", fmt.Errorf("child received unexpected input %q", value)
	}
	return primitiveChildResult, nil
}

func primitiveActivity(_ context.Context, value string) (string, error) {
	if value != primitiveActivityInput {
		return "", fmt.Errorf("activity received unexpected input %q", value)
	}
	return primitiveActivityResult, nil
}

func primitiveHeartbeatActivity(ctx context.Context, value string) (string, error) {
	if value != primitiveHeartbeatInput {
		return "", fmt.Errorf("heartbeat activity received unexpected input %q", value)
	}
	sdkactivity.RecordHeartbeat(ctx, primitiveHeartbeatDetail)
	select {
	case primitiveHeartbeatRecorded <- struct{}{}:
	default:
	}
	select {
	case <-primitiveHeartbeatRelease:
		return primitiveHeartbeatResult, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func primitiveLocalActivity(_ context.Context, value string) (string, error) {
	if value != primitiveLocalInput {
		return "", fmt.Errorf("local activity received unexpected input %q", value)
	}
	return primitiveLocalResult, nil
}

func primitiveFailingActivityFunc(_ context.Context, value string) error {
	if value != primitiveFailureInput {
		return fmt.Errorf("failing activity received unexpected input %q", value)
	}
	return temporal.NewNonRetryableApplicationError(
		"expected test failure",
		"ExpectedFailure",
		nil,
		primitiveFailureDetail,
	)
}

func registerPrimitiveWorker(client sdkclient.Client, taskQueue string) worker.Worker {
	w := worker.New(client, taskQueue, worker.Options{})
	w.RegisterWorkflowWithOptions(primitiveEncryptionWorkflow, workflow.RegisterOptions{Name: primitiveWorkflowType})
	w.RegisterWorkflowWithOptions(primitiveSignalReceiver, workflow.RegisterOptions{Name: primitiveReceiverType})
	w.RegisterWorkflowWithOptions(primitiveChildWorkflow, workflow.RegisterOptions{Name: primitiveChildType})
	w.RegisterActivityWithOptions(primitiveActivity, sdkactivity.RegisterOptions{Name: primitiveActivityType})
	w.RegisterActivityWithOptions(primitiveHeartbeatActivity, sdkactivity.RegisterOptions{Name: primitiveHeartbeatType})
	w.RegisterActivityWithOptions(primitiveLocalActivity, sdkactivity.RegisterOptions{Name: primitiveLocalActivityType})
	w.RegisterActivityWithOptions(primitiveFailingActivityFunc, sdkactivity.RegisterOptions{Name: primitiveFailingActivity})

	service := nexus.NewService(primitiveNexusService)
	service.MustRegister(nexus.NewSyncOperation(
		primitiveNexusOperation,
		func(_ context.Context, input string, _ nexus.StartOperationOptions) (string, error) {
			if input != primitiveNexusInput {
				return "", fmt.Errorf("Nexus operation received unexpected input %q", input)
			}
			return primitiveNexusResult, nil
		},
	))
	w.RegisterNexusService(service)
	return w
}

func primitiveChildExecution(t *testing.T, events []*historypb.HistoryEvent) *commonpb.WorkflowExecution {
	t.Helper()
	for _, event := range events {
		if attributes := event.GetChildWorkflowExecutionStartedEventAttributes(); attributes != nil {
			return attributes.GetWorkflowExecution()
		}
	}
	require.FailNow(t, "history is missing child workflow execution")
	return nil
}

func primitiveContinuedExecution(t *testing.T, workflowID string, events []*historypb.HistoryEvent) *commonpb.WorkflowExecution {
	t.Helper()
	for _, event := range events {
		if attributes := event.GetWorkflowExecutionContinuedAsNewEventAttributes(); attributes != nil {
			return &commonpb.WorkflowExecution{WorkflowId: workflowID, RunId: attributes.GetNewExecutionRunId()}
		}
	}
	require.FailNow(t, "history is missing continued-as-new execution")
	return nil
}

func assertNoPrimitivePlaintext(t *testing.T, message proto.Message) {
	t.Helper()
	wire, err := proto.Marshal(message)
	require.NoError(t, err)
	for _, canary := range primitiveCanaries {
		require.NotContains(t, string(wire), canary)
	}
}

func heartbeatDetailIsSealed(response *adminservice.DescribeMutableStateResponse) bool {
	for _, info := range response.GetDatabaseMutableState().GetActivityInfos() {
		if info.GetActivityType().GetName() != primitiveHeartbeatType {
			continue
		}
		payloads := info.GetLastHeartbeatDetails().GetPayloads()
		if len(payloads) == 0 {
			return false
		}
		for _, payload := range payloads {
			if string(payload.GetMetadata()[proxycodec.MetadataEncoding]) != proxycodec.EncryptionEncoding ||
				bytes.Contains(payload.GetData(), []byte(primitiveHeartbeatDetail)) {
				return false
			}
		}
		return true
	}
	return false
}

func TestWorkerUsesPlaintextForTemporalPrimitivesWhileClusterStoresSealedPayloads(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	cluster := createCluster(log.NewTestLogger(), t, "workflow-primitives", 1, 1, 1)
	t.Cleanup(func() { require.NoError(t, cluster.TearDownCluster()) })
	namespace := registerWorkflowPayloadNamespace(t, cluster)
	proxyAddress := startWorkflowPayloadProxy(t, cluster.Host().FrontendGRPCAddress())

	client, err := sdkclient.Dial(sdkclient.Options{HostPort: proxyAddress, Namespace: namespace})
	require.NoError(t, err)
	t.Cleanup(client.Close)

	taskQueue := "workflow-primitives-encryption"
	endpointName := testcore.RandomizedNexusEndpoint(t.Name())
	endpoint, err := cluster.OperatorClient().CreateNexusEndpoint(ctx, &operatorservice.CreateNexusEndpointRequest{
		Spec: &nexuspb.EndpointSpec{
			Name: endpointName,
			Target: &nexuspb.EndpointTarget{Variant: &nexuspb.EndpointTarget_Worker_{
				Worker: &nexuspb.EndpointTarget_Worker{Namespace: namespace, TaskQueue: taskQueue},
			}},
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = cluster.OperatorClient().DeleteNexusEndpoint(context.Background(), &operatorservice.DeleteNexusEndpointRequest{
			Id:      endpoint.GetEndpoint().GetId(),
			Version: endpoint.GetEndpoint().GetVersion(),
		})
	})

	primitiveHeartbeatRecorded = make(chan struct{}, 1)
	primitiveHeartbeatRelease = make(chan struct{})
	w := registerPrimitiveWorker(client, taskQueue)
	require.NoError(t, w.Start())
	t.Cleanup(w.Stop)

	workflowID := "workflow-primitives-" + common.GenerateRandomString(8)
	receiverRun, err := client.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
		ID:        workflowID + "-receiver",
		TaskQueue: taskQueue,
	}, primitiveReceiverType)
	require.NoError(t, err)

	run, err := client.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
		Memo:      map[string]interface{}{"canary": primitiveMemo},
	}, primitiveWorkflowType, primitiveWorkflowArgs{
		Secret:        primitiveWorkflowInput,
		NexusEndpoint: endpointName,
		ReceiverID:    receiverRun.GetID(),
	})
	require.NoError(t, err)
	initialRunID := run.GetRunID()

	select {
	case <-primitiveHeartbeatRecorded:
	case <-ctx.Done():
		require.NoError(t, ctx.Err())
	}
	require.Eventually(t, func() bool {
		state, stateErr := cluster.AdminClient().DescribeMutableState(ctx, &adminservice.DescribeMutableStateRequest{
			Namespace: namespace,
			Execution: &commonpb.WorkflowExecution{WorkflowId: workflowID, RunId: initialRunID},
		})
		return stateErr == nil && heartbeatDetailIsSealed(state)
	}, 20*time.Second, 100*time.Millisecond, "cluster did not persist the sealed heartbeat detail")
	close(primitiveHeartbeatRelease)

	require.Eventually(t, func() bool {
		value, queryErr := client.QueryWorkflow(ctx, workflowID, initialRunID, primitiveReadyQuery)
		if queryErr != nil {
			return false
		}
		var phase string
		return value.Get(&phase) == nil && phase == "waiting-for-signal"
	}, 30*time.Second, 100*time.Millisecond, "workflow did not reach the signal barrier")

	update, err := client.UpdateWorkflow(ctx, sdkclient.UpdateWorkflowOptions{
		WorkflowID:   workflowID,
		RunID:        initialRunID,
		UpdateName:   primitiveUpdateName,
		Args:         []interface{}{primitiveUpdateInput},
		WaitForStage: sdkclient.WorkflowUpdateStageCompleted,
	})
	require.NoError(t, err)
	var updateResult string
	require.NoError(t, update.Get(ctx, &updateResult))
	require.Equal(t, primitiveUpdateResult, updateResult)

	var receiverResult string
	require.NoError(t, receiverRun.Get(ctx, &receiverResult))
	require.Equal(t, primitiveExternalSignalData, receiverResult)
	require.NoError(t, client.SignalWorkflow(ctx, workflowID, initialRunID, primitiveFinishSignal, primitiveFinishSignalValue))

	var result string
	require.NoError(t, run.Get(ctx, &result))
	require.Equal(t, primitiveContinueResult, result)

	initialExecution := &commonpb.WorkflowExecution{WorkflowId: workflowID, RunId: initialRunID}
	initialHistory := getWorkflowPayloadHistory(t, cluster.FrontendClient(), namespace, initialExecution)
	childExecution := primitiveChildExecution(t, initialHistory.GetEvents())
	continuedExecution := primitiveContinuedExecution(t, workflowID, initialHistory.GetEvents())
	receiverExecution := &commonpb.WorkflowExecution{WorkflowId: receiverRun.GetID(), RunId: receiverRun.GetRunID()}

	directHistories := map[string]*historypb.History{
		"initial":   initialHistory,
		"continued": getWorkflowPayloadHistory(t, cluster.FrontendClient(), namespace, continuedExecution),
		"child":     getWorkflowPayloadHistory(t, cluster.FrontendClient(), namespace, childExecution),
		"receiver":  getWorkflowPayloadHistory(t, cluster.FrontendClient(), namespace, receiverExecution),
	}
	for label, history := range directHistories {
		t.Run("cluster-history-"+label, func(t *testing.T) {
			assertNoPrimitivePlaintext(t, history)
			assertWorkflowPayloadsSealed(t, history, primitiveCanaries)
		})
	}

	state, err := cluster.AdminClient().DescribeMutableState(ctx, &adminservice.DescribeMutableStateRequest{
		Namespace: namespace,
		Execution: initialExecution,
	})
	require.NoError(t, err)
	assertAdminPayloadsSealed(t, state, primitiveCanaries)

	conn, err := grpc.NewClient(proxyAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	proxyClient := workflowservice.NewWorkflowServiceClient(conn)
	proxyHistories := map[string]struct {
		execution *commonpb.WorkflowExecution
		canaries  []string
	}{
		"initial": {
			execution: initialExecution,
			canaries: []string{
				primitiveWorkflowInput,
				primitiveMemo,
				primitiveActivityInput,
				primitiveActivityResult,
				primitiveHeartbeatInput,
				primitiveHeartbeatResult,
				primitiveLocalResult,
				primitiveSideEffectResult,
				primitiveFailureInput,
				primitiveFailureDetail,
				primitiveChildInput,
				primitiveChildResult,
				primitiveNexusInput,
				primitiveNexusResult,
				primitiveFinishSignalValue,
				primitiveExternalSignalData,
				primitiveUpdateInput,
				primitiveUpdateResult,
				primitiveContinueInput,
			},
		},
		"continued": {execution: continuedExecution, canaries: []string{primitiveContinueInput, primitiveContinueResult}},
		"child":     {execution: childExecution, canaries: []string{primitiveChildInput, primitiveChildResult}},
		"receiver":  {execution: receiverExecution, canaries: []string{primitiveExternalSignalData}},
	}
	for label, testCase := range proxyHistories {
		t.Run("proxy-history-"+label, func(t *testing.T) {
			history := getWorkflowPayloadHistory(t, proxyClient, namespace, testCase.execution)
			assertWorkflowPayloadsPlaintext(t, history, testCase.canaries)
		})
	}
}
