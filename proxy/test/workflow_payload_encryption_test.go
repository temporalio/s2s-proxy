package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/codec"
	commonpb "go.temporal.io/api/common/v1"
	historypb "go.temporal.io/api/history/v1"
	apiproxy "go.temporal.io/api/proxy"
	workflowservice "go.temporal.io/api/workflowservice/v1"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/tests/testcore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
	s2sinterceptor "github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/logging"
	s2sproxy "github.com/temporalio/s2s-proxy/proxy"
)

const (
	workflowInputCanary  = "workflow-input-canary"
	activityInputCanary  = "activity-input-canary"
	activityResultCanary = "activity-result-canary"
	memoCanary           = "memo-canary"
)

var workflowPayloadCanaries = []string{
	workflowInputCanary,
	activityInputCanary,
	activityResultCanary,
	memoCanary,
}

type workflowPayloadInput struct {
	Workflow string
	Activity string
}

func workflowPayloadEncryptionWorkflow(ctx workflow.Context, input workflowPayloadInput) (string, error) {
	if input.Workflow != workflowInputCanary {
		return "", fmt.Errorf("workflow received unexpected input %q", input.Workflow)
	}

	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second})
	var activityResult string
	if err := workflow.ExecuteActivity(ctx, workflowPayloadEncryptionActivity, input.Activity).Get(ctx, &activityResult); err != nil {
		return "", err
	}
	if activityResult != activityResultCanary {
		return "", fmt.Errorf("workflow received unexpected activity result %q", activityResult)
	}

	return input.Workflow + "|" + activityResult, nil
}

func workflowPayloadEncryptionActivity(_ context.Context, input string) (string, error) {
	if input != activityInputCanary {
		return "", fmt.Errorf("activity received unexpected input %q", input)
	}
	return activityResultCanary, nil
}

func startWorkflowPayloadProxy(t *testing.T, clusterAddress string) string {
	t.Helper()

	outboundAddress := GetLocalhostAddress()
	inboundAddress := GetLocalhostAddress()
	keyURI := "testing://" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	cfg := config.S2SProxyConfig{ClusterConnections: []config.ClusterConnConfig{{
		Name: "workflow-payload-encryption",
		Local: config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpServer:      config.TCPTLSInfo{ConnectionString: outboundAddress},
			TcpClient:      config.TCPTLSInfo{ConnectionString: clusterAddress},
		},
		Remote: config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpServer:      config.TCPTLSInfo{ConnectionString: inboundAddress},
			TcpClient:      config.TCPTLSInfo{ConnectionString: clusterAddress},
		},
		EncryptionConfig: config.EncryptionConfig{
			Enabled: true,
			Default: &config.KeyPolicy{URI: keyURI, Duration: time.Hour},
		},
	}}}

	provider := config.NewMockConfigProvider(cfg)
	proxy, err := s2sproxy.NewProxy(
		provider,
		logging.NewLoggerProvider(log.NewTestLogger(), provider),
		auth.EmptyCredentialProvider{},
	)
	require.NoError(t, err)
	require.NoError(t, proxy.Start())
	t.Cleanup(proxy.Stop)

	return outboundAddress
}

func registerWorkflowPayloadNamespace(t *testing.T, cluster *testcore.TestCluster) string {
	t.Helper()

	namespace := "workflow-payload-" + common.GenerateRandomString(8)
	_, err := cluster.FrontendClient().RegisterNamespace(t.Context(), &workflowservice.RegisterNamespaceRequest{
		Namespace:                        namespace,
		WorkflowExecutionRetentionPeriod: durationpb.New(24 * time.Hour),
	})
	require.NoError(t, err)

	// The frontend namespace cache is asynchronous. Waiting here keeps the test
	// focused on payload encryption instead of exercising namespace retries.
	time.Sleep(2 * testcore.NamespaceCacheRefreshInterval) //nolint:forbidigo
	return namespace
}

func getWorkflowPayloadHistory(
	t *testing.T,
	client workflowservice.WorkflowServiceClient,
	namespace string,
	execution *commonpb.WorkflowExecution,
) *historypb.History {
	t.Helper()

	history := &historypb.History{}
	request := &workflowservice.GetWorkflowExecutionHistoryRequest{
		Namespace: namespace,
		Execution: execution,
	}
	for {
		response, err := client.GetWorkflowExecutionHistory(t.Context(), request)
		require.NoError(t, err)
		history.Events = append(history.Events, response.GetHistory().GetEvents()...)
		request.NextPageToken = response.GetNextPageToken()
		if len(request.NextPageToken) == 0 {
			return history
		}
	}
}

func assertWorkflowPayloadsSealed(t *testing.T, message proto.Message) {
	t.Helper()

	wire, err := proto.Marshal(message)
	require.NoError(t, err)
	for _, canary := range workflowPayloadCanaries {
		require.NotContains(t, string(wire), canary)
	}

	visited := 0
	require.NoError(t, apiproxy.VisitPayloads(t.Context(), message, apiproxy.VisitPayloadsOptions{
		SkipSearchAttributes: true,
		Visitor: func(
			_ *apiproxy.VisitPayloadsContext,
			payloads []*commonpb.Payload,
		) ([]*commonpb.Payload, error) {
			for _, payload := range payloads {
				visited++
				require.Equal(t, codec.EncryptionEncoding, string(payload.GetMetadata()[codec.MetadataEncoding]))
			}
			return payloads, nil
		},
	}))
	require.Positive(t, visited, "message contained no inspectable payloads")
}

func assertWorkflowPayloadsPlaintext(t *testing.T, history *historypb.History) {
	t.Helper()

	wire, err := proto.Marshal(history)
	require.NoError(t, err)
	for _, canary := range workflowPayloadCanaries {
		require.Contains(t, string(wire), canary)
	}

	visited := 0
	require.NoError(t, apiproxy.VisitPayloads(t.Context(), history, apiproxy.VisitPayloadsOptions{
		SkipSearchAttributes: true,
		Visitor: func(
			_ *apiproxy.VisitPayloadsContext,
			payloads []*commonpb.Payload,
		) ([]*commonpb.Payload, error) {
			for _, payload := range payloads {
				visited++
				require.NotEqual(t, codec.EncryptionEncoding, string(payload.GetMetadata()[codec.MetadataEncoding]))
			}
			return payloads, nil
		},
	}))
	require.Positive(t, visited, "history contained no inspectable payloads")
}

func assertAdminPayloadsSealed(t *testing.T, message proto.Message) {
	t.Helper()

	wire, err := proto.Marshal(message)
	require.NoError(t, err)
	for _, canary := range workflowPayloadCanaries {
		require.NotContains(t, string(wire), canary)
	}

	visited := 0
	require.NoError(t, s2sinterceptor.VisitAdminPayloads(t.Context(), message, s2sinterceptor.AdminVisitOptions{
		Payloads: &apiproxy.VisitPayloadsOptions{
			SkipSearchAttributes: true,
			Visitor: func(
				_ *apiproxy.VisitPayloadsContext,
				payloads []*commonpb.Payload,
			) ([]*commonpb.Payload, error) {
				for _, payload := range payloads {
					visited++
					require.Equal(t, codec.EncryptionEncoding, string(payload.GetMetadata()[codec.MetadataEncoding]))
				}
				return payloads, nil
			},
		},
		Opaque: func(string, []byte) error { return nil },
	}))
	require.Positive(t, visited, "admin response contained no inspectable payloads")
}

func TestWorkerUsesPlaintextWhileClusterStoresSealedPayloads(t *testing.T) {
	cluster := createCluster(log.NewTestLogger(), t, "workflow-payload", 1, 1, 1)
	t.Cleanup(func() { require.NoError(t, cluster.TearDownCluster()) })

	namespace := registerWorkflowPayloadNamespace(t, cluster)
	proxyAddress := startWorkflowPayloadProxy(t, cluster.Host().FrontendGRPCAddress())

	sdkClient, err := sdkclient.Dial(sdkclient.Options{HostPort: proxyAddress, Namespace: namespace})
	require.NoError(t, err)
	t.Cleanup(sdkClient.Close)

	taskQueue := "workflow-payload-encryption"
	w := worker.New(sdkClient, taskQueue, worker.Options{})
	w.RegisterWorkflow(workflowPayloadEncryptionWorkflow)
	w.RegisterActivity(workflowPayloadEncryptionActivity)
	require.NoError(t, w.Start())
	t.Cleanup(w.Stop)

	workflowID := "workflow-payload-" + common.GenerateRandomString(8)
	run, err := sdkClient.ExecuteWorkflow(t.Context(), sdkclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
		Memo:      map[string]interface{}{"canary": memoCanary},
	}, workflowPayloadEncryptionWorkflow, workflowPayloadInput{
		Workflow: workflowInputCanary,
		Activity: activityInputCanary,
	})
	require.NoError(t, err)

	var result string
	require.NoError(t, run.Get(t.Context(), &result))
	require.Equal(t, workflowInputCanary+"|"+activityResultCanary, result)

	execution := &commonpb.WorkflowExecution{WorkflowId: workflowID, RunId: run.GetRunID()}
	directHistory := getWorkflowPayloadHistory(t, cluster.FrontendClient(), namespace, execution)
	assertWorkflowPayloadsSealed(t, directHistory)

	mutableState, err := cluster.AdminClient().DescribeMutableState(t.Context(), &adminservice.DescribeMutableStateRequest{
		Namespace: namespace,
		Execution: execution,
	})
	require.NoError(t, err)
	assertAdminPayloadsSealed(t, mutableState)

	conn, err := grpc.NewClient(proxyAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	proxyHistory := getWorkflowPayloadHistory(t, workflowservice.NewWorkflowServiceClient(conn), namespace, execution)
	assertWorkflowPayloadsPlaintext(t, proxyHistory)
}
