package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	proxycodec "github.com/temporalio/temporal-proxy/pkg/codec"
	commonpb "go.temporal.io/api/common/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/api/workflowservice/v1"
	sdkactivity "go.temporal.io/sdk/activity"
	sdkclient "go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/temporalio/s2s-proxy/config"
	proxymetrics "github.com/temporalio/s2s-proxy/metrics"
)

const (
	migrationReplayWorkflowName = "migration-replay-workflow"
	migrationReplayActivityName = "migration-replay-activity"
	migrationReadyQuery         = "migration-ready"
	migrationContinueSignal     = "migration-continue"
)

type migrationReplayInput struct {
	WorkflowSecret string
	ActivitySecret string
}

func replicationEncryptionConfig() config.EncryptionConfig {
	return config.EncryptionConfig{
		Enabled: true,
		Default: &config.KeyPolicy{
			URI:      testEncryptionKeyURI(1),
			Duration: time.Hour,
		},
	}
}

func testEncryptionKeyURI(value byte) string {
	return "testing://" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{value}, 32))
}

func migrationReplayWorkflow(ctx workflow.Context, input migrationReplayInput) (string, error) {
	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{StartToCloseTimeout: 10 * time.Second})
	phase := "running-on-cluster-a"
	if err := workflow.SetQueryHandler(ctx, migrationReadyQuery, func() (string, error) {
		return phase, nil
	}); err != nil {
		return "", err
	}

	var beforeFailover string
	if err := workflow.ExecuteActivity(ctx, migrationReplayActivityName, input.ActivitySecret).Get(ctx, &beforeFailover); err != nil {
		return "", err
	}

	phase = "waiting-for-failover"
	var signalValue string
	workflow.GetSignalChannel(ctx, migrationContinueSignal).Receive(ctx, &signalValue)

	phase = "replaying-on-cluster-b"
	var afterFailover string
	if err := workflow.ExecuteActivity(ctx, migrationReplayActivityName, signalValue).Get(ctx, &afterFailover); err != nil {
		return "", err
	}

	return strings.Join([]string{input.WorkflowSecret, beforeFailover, signalValue, afterFailover}, "|"), nil
}

func migrationReplayActivity(_ context.Context, value string) (string, error) {
	return "activity-result:" + value, nil
}

func (s *ReplicationTestSuite) TestEncryptedReplicationReplay() {
	if !s.encryptionEnabled {
		s.T().Skip("requires the encrypted replication topology")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	const (
		workflowSecret = "workflow-plaintext-before-migration"
		activitySecret = "activity-plaintext-before-migration"
		signalSecret   = "signal-plaintext-after-failover"
	)
	taskQueue := "encrypted-replication-replay"
	workflowID := "encrypted-replication-" + fmt.Sprint(time.Now().UnixNano())

	clientA := s.newSDKClient(s.clusterA.Host().FrontendGRPCAddress())
	defer clientA.Close()
	workerA := s.newMigrationReplayWorker(clientA, taskQueue, "cluster-a-worker")
	s.Require().NoError(workerA.Start())
	workerAStopped := false
	defer func() {
		if !workerAStopped {
			workerA.Stop()
		}
	}()

	run, err := clientA.ExecuteWorkflow(ctx, sdkclient.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
	}, migrationReplayWorkflowName, migrationReplayInput{
		WorkflowSecret: workflowSecret,
		ActivitySecret: activitySecret,
	})
	s.Require().NoError(err)

	s.Eventually(func() bool {
		value, queryErr := clientA.QueryWorkflow(ctx, workflowID, run.GetRunID(), migrationReadyQuery)
		if queryErr != nil {
			return false
		}
		var phase string
		return value.Get(&phase) == nil && phase == "waiting-for-failover"
	}, 20*time.Second, 200*time.Millisecond, "workflow did not reach the pre-failover barrier")

	workerA.Stop()
	workerAStopped = true

	plainHistory := s.getHistory(ctx, s.clusterA.FrontendClient(), workflowID, run.GetRunID())
	s.assertPayloadsPlaintext(plainHistory, workflowSecret, activitySecret)

	s.waitForClusterSynced()
	sealedHistory := s.getHistory(ctx, s.clusterB.FrontendClient(), workflowID, run.GetRunID())
	s.assertPayloadsSealed(sealedHistory, workflowSecret, activitySecret)

	// Reading Cluster B through the customer-side proxy must reverse exactly the
	// sealing performed on the replication stream.
	proxyClient := s.newSDKClient(s.proxyAOutbound)
	defer proxyClient.Close()
	decryptsBefore := testutil.ToFloat64(proxymetrics.EncryptionDEKOps.WithLabelValues("decrypt", "success"))
	proxyHistory := s.getHistory(ctx, proxyClient.WorkflowService(), workflowID, run.GetRunID())
	s.assertPayloadsPlaintext(proxyHistory, workflowSecret, activitySecret)
	decryptsAfter := testutil.ToFloat64(proxymetrics.EncryptionDEKOps.WithLabelValues("decrypt", "success"))
	s.Greater(decryptsAfter, decryptsBefore, "proxy decrypt-success counter did not increase")
	s.T().Logf(
		"verified proxy decryption: Cluster B stored sealed payloads and returned plaintext through proxy A; decrypt_success_delta=%.0f",
		decryptsAfter-decryptsBefore,
	)

	s.failoverNamespace(ctx, s.namespace, s.clusterB.ClusterName())

	// This is a fresh Worker. Its first workflow task contains the replicated
	// history, so completing proves that replay succeeded after one proxy decode.
	workerB := s.newMigrationReplayWorker(proxyClient, taskQueue, "cluster-b-fresh-worker")
	s.Require().NoError(workerB.Start())
	defer workerB.Stop()

	s.Require().NoError(proxyClient.SignalWorkflow(ctx, workflowID, run.GetRunID(), migrationContinueSignal, signalSecret))
	var result string
	s.Require().NoError(proxyClient.GetWorkflow(ctx, workflowID, run.GetRunID()).Get(ctx, &result))
	s.Equal(strings.Join([]string{
		workflowSecret,
		"activity-result:" + activitySecret,
		signalSecret,
		"activity-result:" + signalSecret,
	}, "|"), result)

	finalSealedHistory := s.getHistory(ctx, s.clusterB.FrontendClient(), workflowID, run.GetRunID())
	s.assertPayloadsSealed(finalSealedHistory, workflowSecret, activitySecret, signalSecret)
	finalProxyHistory := s.getHistory(ctx, proxyClient.WorkflowService(), workflowID, run.GetRunID())
	s.assertPayloadsPlaintext(finalProxyHistory, workflowSecret, activitySecret, signalSecret)

	s.waitForClusterSynced()
	finalPlainHistory := s.getHistory(ctx, s.clusterA.FrontendClient(), workflowID, run.GetRunID())
	s.assertPayloadsPlaintext(finalPlainHistory, workflowSecret, activitySecret, signalSecret)
}

func (s *ReplicationTestSuite) newSDKClient(address string) sdkclient.Client {
	client, err := sdkclient.Dial(sdkclient.Options{
		HostPort:  address,
		Namespace: s.namespace,
	})
	s.Require().NoError(err)
	return client
}

func (s *ReplicationTestSuite) newMigrationReplayWorker(client sdkclient.Client, taskQueue string, identity string) worker.Worker {
	w := worker.New(client, taskQueue, worker.Options{Identity: identity})
	w.RegisterWorkflowWithOptions(migrationReplayWorkflow, workflow.RegisterOptions{Name: migrationReplayWorkflowName})
	w.RegisterActivityWithOptions(migrationReplayActivity, sdkactivity.RegisterOptions{Name: migrationReplayActivityName})
	return w
}

func (s *ReplicationTestSuite) getHistory(
	ctx context.Context,
	client workflowservice.WorkflowServiceClient,
	workflowID string,
	runID string,
) *historypb.History {
	history := &historypb.History{}
	request := &workflowservice.GetWorkflowExecutionHistoryRequest{
		Namespace: s.namespace,
		Execution: &commonpb.WorkflowExecution{WorkflowId: workflowID, RunId: runID},
	}
	for {
		response, err := client.GetWorkflowExecutionHistory(ctx, request)
		s.NoError(err)
		if err != nil {
			return history
		}
		history.Events = append(history.Events, response.GetHistory().GetEvents()...)
		request.NextPageToken = response.GetNextPageToken()
		if len(request.NextPageToken) == 0 {
			return history
		}
	}
}

func historyApplicationPayloads(history *historypb.History) []*commonpb.Payload {
	var payloads []*commonpb.Payload
	appendPayloads := func(values *commonpb.Payloads) {
		payloads = append(payloads, values.GetPayloads()...)
	}

	for _, event := range history.GetEvents() {
		if attributes := event.GetWorkflowExecutionStartedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
		}
		if attributes := event.GetActivityTaskScheduledEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
		}
		if attributes := event.GetActivityTaskCompletedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetResult())
		}
		if attributes := event.GetWorkflowExecutionSignaledEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetInput())
		}
		if attributes := event.GetWorkflowExecutionCompletedEventAttributes(); attributes != nil {
			appendPayloads(attributes.GetResult())
		}
	}
	return payloads
}

func (s *ReplicationTestSuite) assertPayloadsSealed(history *historypb.History, plaintextValues ...string) {
	payloads := historyApplicationPayloads(history)
	s.NotEmpty(payloads, "history contained no application payloads")
	for _, payload := range payloads {
		metadata := payload.GetMetadata()
		s.Equal(proxycodec.EncryptionEncoding, string(metadata[proxycodec.MetadataEncoding]))
		s.NotEmpty(metadata[proxycodec.MetadataEncryptionKeyID])
		s.NotEmpty(metadata[proxycodec.MetadataEncryptionDEK])
		for _, plaintext := range plaintextValues {
			s.NotContains(string(payload.GetData()), plaintext)
		}
	}
}

func (s *ReplicationTestSuite) assertPayloadsPlaintext(history *historypb.History, plaintextValues ...string) {
	payloads := historyApplicationPayloads(history)
	s.NotEmpty(payloads, "history contained no application payloads")
	var data []byte
	for _, payload := range payloads {
		metadata := payload.GetMetadata()
		s.Equal("json/plain", string(metadata[proxycodec.MetadataEncoding]))
		s.Empty(metadata[proxycodec.MetadataEncryptionKeyID])
		s.Empty(metadata[proxycodec.MetadataEncryptionDEK])
		data = append(data, payload.GetData()...)
	}
	for _, plaintext := range plaintextValues {
		s.Contains(string(data), plaintext)
	}
}
