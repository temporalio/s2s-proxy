package interceptor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/errordetails/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/proxy/pkg/crypto"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	persistence "go.temporal.io/server/api/persistence/v1"
	replication "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/service/history/tasks"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/config"
)

// recordingObserver remembers what it was told, as "kind path".
type recordingObserver struct {
	mu               sync.Mutex
	passed, rejected []string
}

func (o *recordingObserver) OpaquePassed(kind, path string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.passed = append(o.passed, kind+" "+path)
}

func (o *recordingObserver) OpaqueRejected(kind, path string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.rejected = append(o.rejected, kind+" "+path)
}

func TestNewAdminEncryptor(t *testing.T) {
	t.Run("encryption enabled without a vault is refused", func(t *testing.T) {
		e, err := NewAdminEncryptor(AdminEncryptorConfig{EncryptorConfig: EncryptorConfig{Enabled: true}})
		require.Nil(t, e)
		require.ErrorContains(t, err, "encryption requires a vault")
	})

	t.Run("no vault does nothing", func(t *testing.T) {
		e := requireAdminEncryptor(t, AdminEncryptorConfig{})
		reply := dlqReply(activityTask(payloads("plain")))

		require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))
		require.Equal(t, []string{"plain"}, data(activityDetails(reply)))
	})
}

func TestAdminEncryptorUnaryRoundTrip(t *testing.T) {
	v := &fakeVault{}
	e := requireAdminEncryptor(t, adminConfig(v))

	reply := dlqReply(activityTask(sealed(t, "from-peer")))
	require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))

	require.Zero(t, sealedCount(activityDetails(reply)), "responses are opened")
	require.Equal(t, []string{"from-peer"}, data(activityDetails(reply)))
}

func TestAdminEncryptorSealsUnderTheReplicationNamespace(t *testing.T) {
	v := &fakeVault{}
	e := requireAdminEncryptor(t, reverseConfig(v))

	// A namespace on the context, as StampNamespace may leave one, must not
	// pick the key: admin traffic always seals under the replication namespace.
	ctx := context.WithValue(t.Context(), NamespaceKey, "some-namespace")
	reply := dlqReply(activityTask(payloads("a", "b")))

	require.NoError(t, adminCallWith(ctx, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))
	require.Equal(t, []string{config.ReplicationKeyNamespace, config.ReplicationKeyNamespace}, v.namespaces())
}

func TestAdminEncryptorReverse(t *testing.T) {
	// The client facing the local cluster: what the local cluster returns is
	// sealed on its way to the peer.
	v := &fakeVault{}
	e := requireAdminEncryptor(t, reverseConfig(v))

	reply := dlqReply(activityTask(payloads("local")))
	require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))

	require.Equal(t, 1, sealedCount(activityDetails(reply)))
	require.NotContains(t, data(activityDetails(reply)), "local")
}

func TestAdminEncryptorMixedPayloads(t *testing.T) {
	// History replicated back from Cloud: some payloads were sealed by the
	// workflow path under a namespace key, others were never sealed.
	v := &fakeVault{}
	e := requireAdminEncryptor(t, reverseConfig(v))

	mixed := &common.Payloads{Payloads: append(sealed(t, "already").Payloads, payloads("plain").Payloads...)}
	reply := dlqReply(activityTask(mixed))

	require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))
	require.Equal(t, 2, sealedCount(activityDetails(reply)))
	require.Equal(t, []string{config.ReplicationKeyNamespace}, v.namespaces(), "only the plaintext payload is sealed")

	// And the peer opens both.
	opener := requireAdminEncryptor(t, adminConfig(&fakeVault{}))
	require.NoError(t, adminCall(t, opener.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))
	require.Equal(t, []string{"already", "plain"}, data(activityDetails(reply)))
}

func TestAdminEncryptorLeavesRoutingInfoAlone(t *testing.T) {
	e := requireAdminEncryptor(t, reverseConfig(&fakeVault{}))

	task := activityTask(payloads("x"))
	task.RawTaskInfo = &persistence.ReplicationTaskInfo{NamespaceId: "ns-id", WorkflowId: "wf-id"}
	want := proto.Clone(task.RawTaskInfo)

	reply := dlqReply(task)
	require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))

	require.True(t, proto.Equal(want, reply.GetReplicationTasks()[0].GetRawTaskInfo()))
	require.Equal(t, 1, sealedCount(activityDetails(reply)), "the payload beside it was sealed")
}

func TestAdminEncryptorOpaque(t *testing.T) {
	hsm := func() *adminservice.GetDLQReplicationMessagesResponse {
		return dlqReply(hsmTask("hsm-state"))
	}

	t.Run("sealing refuses it by default", func(t *testing.T) {
		obs := &recordingObserver{}
		cfg := reverseConfig(&fakeVault{})
		cfg.Observer = obs
		e := requireAdminEncryptor(t, cfg)

		err := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, hsm(), func() error { return nil })
		requireGenericVisitError(t, err, "allowOpaquePlaintext")
		require.NotContains(t, err.Error(), "hsm-state", "the data stays out of the error")
		require.Len(t, obs.rejected, 1)
		require.Empty(t, obs.passed)
	})

	t.Run("sealing lets it through when allowed", func(t *testing.T) {
		obs := &recordingObserver{}
		cfg := reverseConfig(&fakeVault{})
		cfg.AllowOpaquePlaintext = true
		cfg.Observer = obs
		e := requireAdminEncryptor(t, cfg)

		reply := hsm()
		require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))
		require.Equal(t, "hsm-state", string(reply.GetReplicationTasks()[0].GetSyncHsmAttributes().GetStateMachineNode().GetData()))
		require.Len(t, obs.passed, 1)
		require.Contains(t, obs.passed[0], "hsm ")
		require.Empty(t, obs.rejected)
	})

	t.Run("opening ignores it", func(t *testing.T) {
		obs := &recordingObserver{}
		cfg := adminConfig(&fakeVault{})
		cfg.Observer = obs
		e := requireAdminEncryptor(t, cfg)

		require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, hsm(), func() error { return nil }))
		require.Empty(t, obs.passed)
		require.Empty(t, obs.rejected)
	})
}

func TestAdminEncryptorVaultFailure(t *testing.T) {
	boom := errors.New("kms down")
	e := requireAdminEncryptor(t, AdminEncryptorConfig{EncryptorConfig: EncryptorConfig{Enabled: true, Vault: &fakeVault{sealErr: boom}}})

	// ImportWorkflowExecution carries history in its request, so sealing it
	// actually reaches the vault.
	req := &adminservice.ImportWorkflowExecutionRequest{
		HistoryBatches: []*common.DataBlob{eventBlob(t, payloads("x"))},
	}
	invoked := false

	err := adminCall(t, e.Unary, req, new(adminservice.ImportWorkflowExecutionResponse), func() error {
		invoked = true
		return nil
	})
	require.ErrorIs(t, err, boom)
	require.False(t, invoked, "nothing is sent when sealing fails")
}

func TestAdminEncryptorErrorDetails(t *testing.T) {
	e := requireAdminEncryptor(t, adminConfig(&fakeVault{}))

	st, err := status.New(codes.FailedPrecondition, "nope").WithDetails(&errordetails.QueryFailedFailure{
		Failure: &failure.Failure{FailureInfo: &failure.Failure_ApplicationFailureInfo{
			ApplicationFailureInfo: &failure.ApplicationFailureInfo{Details: sealed(t, "detail")},
		}},
	})
	require.NoError(t, err)

	got := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, new(adminservice.ReapplyEventsResponse), func() error {
		return st.Err()
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(got))

	details := status.Convert(got).Details()
	require.Len(t, details, 1)
	qf, ok := details[0].(*errordetails.QueryFailedFailure)
	require.True(t, ok)
	require.Equal(t, []string{"detail"}, data(qf.GetFailure().GetApplicationFailureInfo().GetDetails()))
}

func TestAdminEncryptorFailures(t *testing.T) {
	want := appFailure("secret message", "secret stack")
	want.Cause = appFailure("secret cause", "secret cause stack")
	clone := func() *failure.Failure { return proto.Clone(want).(*failure.Failure) }
	withFailures := func(cfg AdminEncryptorConfig) AdminEncryptorConfig {
		cfg.EncodeFailures = true
		return cfg
	}

	t.Run("failures in event blobs are sealed on the way out", func(t *testing.T) {
		e := requireAdminEncryptor(t, withFailures(adminConfig(&fakeVault{})))
		req := &adminservice.ImportWorkflowExecutionRequest{
			HistoryBatches: []*common.DataBlob{failedEventBlob(t, clone())},
		}

		require.NoError(t, adminCall(t, e.Unary, req, new(adminservice.ImportWorkflowExecutionResponse), func() error {
			f := failureInBlob(t, req.GetHistoryBatches()[0])
			requireFailureSealed(t, f)
			requireFailureSealed(t, f.GetCause())
			return nil
		}))
	})

	t.Run("failures in event blobs from the peer are restored", func(t *testing.T) {
		// Reverse opens requests: the peer's history arriving for the local cluster.
		e := requireAdminEncryptor(t, reverseConfig(&fakeVault{}))
		req := &adminservice.ImportWorkflowExecutionRequest{
			HistoryBatches: []*common.DataBlob{failedEventBlob(t, sealedFailure(t, want))},
		}

		require.NoError(t, adminCall(t, e.Unary, req, new(adminservice.ImportWorkflowExecutionResponse), func() error {
			got := failureInBlob(t, req.GetHistoryBatches()[0])
			require.True(t, proto.Equal(want, got), "local cluster got %v", got)
			return nil
		}))
	})

	t.Run("failures in replication tasks round trip", func(t *testing.T) {
		sealer := requireAdminEncryptor(t, withFailures(reverseConfig(&fakeVault{})))
		reply := dlqReply(failedActivityTask(clone()))
		require.NoError(t, adminCall(t, sealer.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))

		onWire := reply.GetReplicationTasks()[0].GetSyncActivityTaskAttributes().GetLastFailure()
		requireFailureSealed(t, onWire)
		requireFailureSealed(t, onWire.GetCause())

		opener := requireAdminEncryptor(t, adminConfig(&fakeVault{}))
		back := dlqReply(failedActivityTask(onWire))
		require.NoError(t, adminCall(t, opener.Unary, &adminservice.ReapplyEventsRequest{}, back, func() error { return nil }))

		got := back.GetReplicationTasks()[0].GetSyncActivityTaskAttributes().GetLastFailure()
		require.True(t, proto.Equal(want, got), "got %v", got)
	})

	t.Run("flag off leaves failures in plaintext", func(t *testing.T) {
		e := requireAdminEncryptor(t, reverseConfig(&fakeVault{}))
		reply := dlqReply(failedActivityTask(clone()))
		require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))

		got := reply.GetReplicationTasks()[0].GetSyncActivityTaskAttributes().GetLastFailure()
		require.True(t, proto.Equal(want, got), "got %v", got)
	})

	t.Run("streams seal and restore", func(t *testing.T) {
		sealer := requireAdminEncryptor(t, withFailures(reverseConfig(&fakeVault{})))
		cs := openStream(t, sealer, &fakeClientStream{ctx: t.Context(), err: io.EOF,
			recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{streamMessages(failedActivityTask(clone()))}})

		sealedMsg := new(adminservice.StreamWorkflowReplicationMessagesResponse)
		require.NoError(t, cs.RecvMsg(sealedMsg))
		requireFailureSealed(t, sealedMsg.GetMessages().GetReplicationTasks()[0].GetSyncActivityTaskAttributes().GetLastFailure())

		opener := requireAdminEncryptor(t, adminConfig(&fakeVault{}))
		cs = openStream(t, opener, &fakeClientStream{ctx: t.Context(), err: io.EOF,
			recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{sealedMsg}})

		got := new(adminservice.StreamWorkflowReplicationMessagesResponse)
		require.NoError(t, cs.RecvMsg(got))
		lf := got.GetMessages().GetReplicationTasks()[0].GetSyncActivityTaskAttributes().GetLastFailure()
		require.True(t, proto.Equal(want, lf), "got %v", lf)
	})

	t.Run("failures in error details are restored", func(t *testing.T) {
		e := requireAdminEncryptor(t, adminConfig(&fakeVault{}))

		st, err := status.New(codes.FailedPrecondition, "nope").
			WithDetails(&errordetails.QueryFailedFailure{Failure: sealedFailure(t, want)})
		require.NoError(t, err)

		got := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, new(adminservice.ReapplyEventsResponse), st.Err)
		require.Equal(t, codes.FailedPrecondition, status.Code(got))

		details := status.Convert(got).Details()
		require.Len(t, details, 1)
		qf, ok := details[0].(*errordetails.QueryFailedFailure)
		require.True(t, ok)
		require.True(t, proto.Equal(want, qf.GetFailure()), "got %v", qf.GetFailure())
	})

	t.Run("reverse seals failures in error details", func(t *testing.T) {
		e := requireAdminEncryptor(t, withFailures(reverseConfig(&fakeVault{})))

		st, err := status.New(codes.FailedPrecondition, "nope").
			WithDetails(&errordetails.QueryFailedFailure{Failure: clone()})
		require.NoError(t, err)

		got := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, new(adminservice.ReapplyEventsResponse), st.Err)
		require.Equal(t, codes.FailedPrecondition, status.Code(got))

		details := status.Convert(got).Details()
		require.Len(t, details, 1)
		qf, ok := details[0].(*errordetails.QueryFailedFailure)
		require.True(t, ok)
		requireFailureSealed(t, qf.GetFailure())
		requireFailureSealed(t, qf.GetFailure().GetCause())
	})

	t.Run("failure encoding without encryption is refused", func(t *testing.T) {
		e, err := NewAdminEncryptor(AdminEncryptorConfig{EncryptorConfig: EncryptorConfig{Vault: &fakeVault{}, EncodeFailures: true}})
		require.Nil(t, e)
		require.ErrorContains(t, err, "failure encoding requires encryption")
	})
}

// failedActivityTask is a replication task carrying f as the activity's last
// failure.
func failedActivityTask(f *failure.Failure) *replication.ReplicationTask {
	return &replication.ReplicationTask{
		Attributes: &replication.ReplicationTask_SyncActivityTaskAttributes{
			SyncActivityTaskAttributes: &replication.SyncActivityTaskAttributes{LastFailure: f},
		},
	}
}

// failedEventBlob serializes one activity task failed event carrying f.
func failedEventBlob(t *testing.T, f *failure.Failure) *common.DataBlob {
	t.Helper()

	blob, err := serializer.SerializeEvents([]*history.HistoryEvent{{
		EventId:   1,
		EventType: enums.EVENT_TYPE_ACTIVITY_TASK_FAILED,
		Attributes: &history.HistoryEvent_ActivityTaskFailedEventAttributes{
			ActivityTaskFailedEventAttributes: &history.ActivityTaskFailedEventAttributes{Failure: f},
		},
	}})
	require.NoError(t, err)

	return blob
}

// failureInBlob returns the failure of the single event failedEventBlob wrote.
func failureInBlob(t *testing.T, blob *common.DataBlob) *failure.Failure {
	t.Helper()

	events, err := serializer.DeserializeEvents(blob)
	require.NoError(t, err)
	require.Len(t, events, 1)

	return events[0].GetActivityTaskFailedEventAttributes().GetFailure()
}

// activityTask is a replication task carrying details as activity heartbeat
// details, the simplest server-internal home for a Payloads.
func activityTask(details *common.Payloads) *replication.ReplicationTask {
	return &replication.ReplicationTask{
		Attributes: &replication.ReplicationTask_SyncActivityTaskAttributes{
			SyncActivityTaskAttributes: &replication.SyncActivityTaskAttributes{Details: details},
		},
	}
}

// hsmTask is a replication task carrying a state machine node, which the
// visitor reports as opaque.
func hsmTask(state string) *replication.ReplicationTask {
	return &replication.ReplicationTask{
		Attributes: &replication.ReplicationTask_SyncHsmAttributes{
			SyncHsmAttributes: &replication.SyncHSMAttributes{
				StateMachineNode: &persistence.StateMachineNode{Data: []byte(state)},
			},
		},
	}
}

func dlqReply(tasks ...*replication.ReplicationTask) *adminservice.GetDLQReplicationMessagesResponse {
	return &adminservice.GetDLQReplicationMessagesResponse{ReplicationTasks: tasks}
}

// activityDetails returns the details of the first task in r.
func activityDetails(r *adminservice.GetDLQReplicationMessagesResponse) *common.Payloads {
	return r.GetReplicationTasks()[0].GetSyncActivityTaskAttributes().GetDetails()
}

// eventBlob serializes one workflow started event whose input is ps.
func eventBlob(t *testing.T, ps *common.Payloads) *common.DataBlob {
	t.Helper()

	blob, err := serializer.SerializeEvents([]*history.HistoryEvent{{
		EventId:   1,
		EventType: enums.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
		Attributes: &history.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &history.WorkflowExecutionStartedEventAttributes{Input: ps},
		},
	}})
	require.NoError(t, err)

	return blob
}

func adminConfig(v Vault) AdminEncryptorConfig {
	return AdminEncryptorConfig{EncryptorConfig: EncryptorConfig{Enabled: true, Vault: v}}
}

// reverseConfig is the config of the client facing the local cluster, which
// seals responses.
func reverseConfig(v Vault) AdminEncryptorConfig {
	return AdminEncryptorConfig{EncryptorConfig: EncryptorConfig{Enabled: true, Vault: v, Reverse: true}}
}

func requireAdminEncryptor(t *testing.T, cfg AdminEncryptorConfig) *AdminEncryptor {
	t.Helper()

	e, err := NewAdminEncryptor(cfg)
	require.NoError(t, err)
	require.NotNil(t, e)

	return e
}

func adminCall(t *testing.T, unary grpc.UnaryClientInterceptor, req, reply any, invoke func() error) error {
	t.Helper()
	return adminCallWith(t.Context(), unary, req, reply, invoke)
}

func adminCallWith(ctx context.Context, unary grpc.UnaryClientInterceptor, req, reply any, invoke func() error) error {
	return unary(ctx, "/temporal.server.api.adminservice.v1.AdminService/Test", req, reply, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return invoke()
		},
	)
}

// fakeClientStream hands out queued messages from RecvMsg and records what
// SendMsg was given.
type fakeClientStream struct {
	grpc.ClientStream
	ctx  context.Context
	recv []*adminservice.StreamWorkflowReplicationMessagesResponse
	err  error // returned once recv is drained
	sent []proto.Message
}

func (s *fakeClientStream) Context() context.Context { return s.ctx }

func (s *fakeClientStream) SendMsg(m any) error {
	s.sent = append(s.sent, proto.Clone(m.(proto.Message)))
	return nil
}

func (s *fakeClientStream) RecvMsg(m any) error {
	if len(s.recv) == 0 {
		return s.err
	}

	next := s.recv[0]
	s.recv = s.recv[1:]
	proto.Merge(m.(proto.Message), next)
	return nil
}

func TestAdminEncryptorStream(t *testing.T) {
	t.Run("received tasks are opened", func(t *testing.T) {
		e := requireAdminEncryptor(t, adminConfig(&fakeVault{}))
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: io.EOF,
			recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{streamMessages(activityTask(sealed(t, "from-peer")))}})

		got := new(adminservice.StreamWorkflowReplicationMessagesResponse)
		require.NoError(t, cs.RecvMsg(got))
		require.Equal(t, []string{"from-peer"}, data(streamDetails(got)))
	})

	t.Run("reverse seals received tasks", func(t *testing.T) {
		v := &fakeVault{}
		e := requireAdminEncryptor(t, reverseConfig(v))
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: io.EOF,
			recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{streamMessages(activityTask(payloads("local")))}})

		got := new(adminservice.StreamWorkflowReplicationMessagesResponse)
		require.NoError(t, cs.RecvMsg(got))
		require.Equal(t, 1, sealedCount(streamDetails(got)))
		require.Equal(t, []string{config.ReplicationKeyNamespace}, v.namespaces())
	})

	t.Run("EOF comes through as EOF", func(t *testing.T) {
		e := requireAdminEncryptor(t, adminConfig(&fakeVault{}))
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: io.EOF})

		err := cs.RecvMsg(new(adminservice.StreamWorkflowReplicationMessagesResponse))
		require.Same(t, io.EOF, err) //nolint:errorlint // identity is the point: callers compare with ==
	})

	t.Run("a failed visit fails RecvMsg", func(t *testing.T) {
		e := requireAdminEncryptor(t, reverseConfig(&fakeVault{}))
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: io.EOF,
			recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{streamMessages(hsmTask("x"))}})

		err := cs.RecvMsg(new(adminservice.StreamWorkflowReplicationMessagesResponse))
		require.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("acks pass through unchanged", func(t *testing.T) {
		e := requireAdminEncryptor(t, adminConfig(&fakeVault{}))
		fake := &fakeClientStream{ctx: t.Context()}
		cs := openStream(t, e, fake)

		ack := &adminservice.StreamWorkflowReplicationMessagesRequest{
			Attributes: &adminservice.StreamWorkflowReplicationMessagesRequest_SyncReplicationState{
				SyncReplicationState: &replication.SyncReplicationState{InclusiveLowWatermark: 42},
			},
		}
		require.NoError(t, cs.SendMsg(ack))
		require.Len(t, fake.sent, 1)
		require.True(t, proto.Equal(ack, fake.sent[0]))
	})

	t.Run("no vault returns the stream untouched", func(t *testing.T) {
		e := requireAdminEncryptor(t, AdminEncryptorConfig{})
		fake := &fakeClientStream{ctx: t.Context()}
		require.Same(t, fake, openStream(t, e, fake))
	})
}

func openStream(t *testing.T, e *AdminEncryptor, fake *fakeClientStream) grpc.ClientStream {
	t.Helper()

	cs, err := e.Stream(t.Context(), &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, nil,
		"/temporal.server.api.adminservice.v1.AdminService/StreamWorkflowReplicationMessages",
		func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
			return fake, nil
		})
	require.NoError(t, err)

	return cs
}

func streamMessages(tasks ...*replication.ReplicationTask) *adminservice.StreamWorkflowReplicationMessagesResponse {
	return &adminservice.StreamWorkflowReplicationMessagesResponse{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
			Messages: &replication.WorkflowReplicationMessages{ReplicationTasks: tasks},
		},
	}
}

// streamDetails returns the activity details of the first task in r.
func streamDetails(r *adminservice.StreamWorkflowReplicationMessagesResponse) *common.Payloads {
	return r.GetMessages().GetReplicationTasks()[0].GetSyncActivityTaskAttributes().GetDetails()
}

func TestAdminEncryptorAddTasks(t *testing.T) {
	// When state-based replication falls back to events, the receiving cluster
	// hands the source its task equivalents through AddTasks. Sealing must let
	// those replication tasks through, and keep refusing the HSM or CHASM data
	// other task categories can carry.
	replicationBlob, err := serializer.ReplicationTaskInfoToBlob(&persistence.ReplicationTaskInfo{
		NamespaceId: "ns-id",
		WorkflowId:  "wf-id",
		TaskId:      7,
	})
	require.NoError(t, err)

	addTasks := func(category int32, blob *common.DataBlob) *adminservice.AddTasksRequest {
		return &adminservice.AddTasksRequest{
			ShardId: 1,
			Tasks:   []*adminservice.AddTasksRequest_Task{{CategoryId: category, Blob: blob}},
		}
	}

	t.Run("replication tasks are sent as they are", func(t *testing.T) {
		obs := &recordingObserver{}
		cfg := adminConfig(&fakeVault{})
		cfg.Observer = obs
		e := requireAdminEncryptor(t, cfg)

		req := addTasks(int32(tasks.CategoryIDReplication), replicationBlob)
		want := proto.Clone(req)
		invoked := false

		err := adminCall(t, e.Unary, req, new(adminservice.AddTasksResponse), func() error {
			invoked = true
			return nil
		})
		require.NoError(t, err)
		require.True(t, invoked)
		require.True(t, proto.Equal(want, req))
		require.Empty(t, obs.rejected)
		require.Empty(t, obs.passed, "nothing opaque was sent, so nothing is reported")
	})

	t.Run("tasks of other categories are refused only for the CHASM data in them", func(t *testing.T) {
		withChasm, err := serializer.TransferTaskInfoToBlob(&persistence.TransferTaskInfo{
			TaskDetails: &persistence.TransferTaskInfo_ChasmTaskInfo{
				ChasmTaskInfo: &persistence.ChasmTaskInfo{Data: &common.DataBlob{Data: []byte("component")}},
			},
		})
		require.NoError(t, err)
		without, err := serializer.TransferTaskInfoToBlob(&persistence.TransferTaskInfo{WorkflowId: "wf-id"})
		require.NoError(t, err)

		obs := &recordingObserver{}
		cfg := adminConfig(&fakeVault{})
		cfg.Observer = obs
		e := requireAdminEncryptor(t, cfg)

		err = adminCall(t, e.Unary, addTasks(int32(tasks.CategoryIDTransfer), withChasm),
			new(adminservice.AddTasksResponse), func() error { return nil })
		requireGenericVisitError(t, err, "cannot seal opaque chasm data")
		require.Equal(t, []string{"chasm TransferTaskInfo/TaskDetails/ChasmTaskInfo/ChasmTaskInfo/Data"}, obs.rejected)

		err = adminCall(t, e.Unary, addTasks(int32(tasks.CategoryIDTransfer), without),
			new(adminservice.AddTasksResponse), func() error { return nil })
		require.NoError(t, err)
	})

	t.Run("a replication blob that is not a replication task is refused", func(t *testing.T) {
		e := requireAdminEncryptor(t, adminConfig(&fakeVault{}))
		garbage := &common.DataBlob{EncodingType: enums.ENCODING_TYPE_PROTO3, Data: []byte{0xff, 0xff, 0xff}}

		err := adminCall(t, e.Unary, addTasks(int32(tasks.CategoryIDReplication), garbage),
			new(adminservice.AddTasksResponse), func() error { return nil })
		requireGenericVisitError(t, err, "cannot seal opaque task data")
	})
}

func TestAdminEncryptorBindsToTheReplicationNamespace(t *testing.T) {
	// Admin traffic has no label func, so it is bound to the replication key's
	// name whatever namespace the context carries, and a strict vault still opens
	// it, since nothing is compared on open.
	ctx := context.WithValue(t.Context(), NamespaceKey, "some-namespace")
	reply := dlqReply(activityTask(payloads("a")))

	seal := requireAdminEncryptor(t, reverseConfig(realVault(t, crypto.BindingPermissive)))
	require.NoError(t, adminCallWith(ctx, seal.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))
	require.Equal(t, []string{config.ReplicationKeyNamespace}, boundTo(activityDetails(reply)))

	obs := &recordingBindingObserver{}
	cfg := adminConfig(realVault(t, crypto.BindingStrict))
	cfg.Binding = obs
	open := requireAdminEncryptor(t, cfg)

	require.NoError(t, adminCallWith(ctx, open.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))
	require.Equal(t, []string{"a"}, data(activityDetails(reply)))
	require.Empty(t, obs.unbound)
}

// fakeNamespaces resolves IDs from a map, or fails with err when it is set,
// counting the lookups it serves.
type fakeNamespaces struct {
	mu    sync.Mutex
	names map[string]string
	err   error
	calls int
}

func (f *fakeNamespaces) LocalName(_ context.Context, id string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.err != nil {
		return "", f.err
	}

	name, ok := f.names[id]
	if !ok {
		return "", errors.New("no namespace with ID " + id)
	}

	return name, nil
}

// activityTaskIn is activityTask for the namespace with ID id.
func activityTaskIn(id string, details *common.Payloads) *replication.ReplicationTask {
	task := activityTask(details)
	task.GetSyncActivityTaskAttributes().NamespaceId = id
	return task
}

// namespacedConfig is cfg resolving IDs through names and labeling with
// peerLabel.
func namespacedConfig(cfg AdminEncryptorConfig, names NamespaceResolver) AdminEncryptorConfig {
	cfg.Label = peerLabel
	cfg.Namespaces = names
	return cfg
}

func TestAdminEncryptorBindsEachTaskToItsNamespace(t *testing.T) {
	names := &fakeNamespaces{names: map[string]string{"id-a": "a", "id-b": "b"}}
	e := requireAdminEncryptor(t, namespacedConfig(reverseConfig(realVault(t, crypto.BindingPermissive)), names))

	reply := dlqReply(activityTaskIn("id-a", payloads("1")), activityTaskIn("id-b", payloads("2")))
	require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))

	got := reply.GetReplicationTasks()
	require.Equal(t, []string{"peer-a"}, boundTo(got[0].GetSyncActivityTaskAttributes().GetDetails()))
	require.Equal(t, []string{"peer-b"}, boundTo(got[1].GetSyncActivityTaskAttributes().GetDetails()))
}

func TestAdminEncryptorChecksEachTaskOnOpen(t *testing.T) {
	names := &fakeNamespaces{names: map[string]string{"id-a": "a"}}
	sealer := realVault(t, crypto.BindingPermissive)

	open := func(t *testing.T, mode crypto.BindingMode, held *common.Payloads) (*adminservice.GetDLQReplicationMessagesResponse, *recordingBindingObserver, error) {
		obs := &recordingBindingObserver{}
		cfg := namespacedConfig(adminConfig(realVault(t, mode)), names)
		cfg.Binding = obs
		e := requireAdminEncryptor(t, cfg)

		reply := new(adminservice.GetDLQReplicationMessagesResponse)
		err := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error {
			reply.ReplicationTasks = []*replication.ReplicationTask{activityTaskIn("id-a", held)}
			return nil
		})

		return reply, obs, err
	}

	t.Run("strict opens a payload bound to the task's label", func(t *testing.T) {
		reply, obs, err := open(t, crypto.BindingStrict, sealedFor(t, sealer, "peer-a", "mine"))
		require.NoError(t, err)
		require.Equal(t, []string{"mine"}, data(activityDetails(reply)))
		require.Empty(t, obs.mismatched)
	})

	t.Run("strict refuses one bound to the replication key and counts it under the local name", func(t *testing.T) {
		_, obs, err := open(t, crypto.BindingStrict, sealedFor(t, sealer, config.ReplicationKeyNamespace, "old"))
		require.ErrorIs(t, err, crypto.ErrNamespaceMismatch)
		require.Equal(t, []string{"a"}, obs.mismatched)
	})
}

// recordingNamespaceObserver remembers the IDs it is told failed to resolve.
type recordingNamespaceObserver struct {
	mu  sync.Mutex
	ids []string
}

func (o *recordingNamespaceObserver) UnresolvedNamespace(id string, _ error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.ids = append(o.ids, id)
}

func TestAdminEncryptorUnresolvedNamespace(t *testing.T) {
	down := errors.New("frontend unavailable")

	t.Run("falls back to the replication key and reports it", func(t *testing.T) {
		obs := &recordingNamespaceObserver{}
		cfg := namespacedConfig(reverseConfig(realVault(t, crypto.BindingPermissive)), &fakeNamespaces{err: down})
		cfg.NamespaceObserver = obs
		e := requireAdminEncryptor(t, cfg)

		reply := dlqReply(activityTaskIn("id-a", payloads("1")))
		require.NoError(t, adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil }))
		require.Equal(t, []string{config.ReplicationKeyNamespace}, boundTo(activityDetails(reply)))
		require.Equal(t, []string{"id-a"}, obs.ids)
	})

	t.Run("strict fails the call and reports it", func(t *testing.T) {
		obs := &recordingNamespaceObserver{}
		cfg := namespacedConfig(reverseConfig(realVault(t, crypto.BindingStrict)), &fakeNamespaces{err: down})
		cfg.StrictNamespaces = true
		cfg.NamespaceObserver = obs
		e := requireAdminEncryptor(t, cfg)

		reply := dlqReply(activityTaskIn("id-a", payloads("1")))
		err := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error { return nil })
		require.ErrorContains(t, err, "frontend unavailable")
		require.ErrorContains(t, err, "id-a")
		require.Equal(t, []string{"1"}, data(activityDetails(reply)), "nothing is sealed")
		require.Equal(t, []string{"id-a"}, obs.ids)
	})
}

// inputInBlob returns the input of the single event eventBlob wrote.
func inputInBlob(t *testing.T, blob *common.DataBlob) *common.Payloads {
	t.Helper()

	events, err := serializer.DeserializeEvents(blob)
	require.NoError(t, err)
	require.Len(t, events, 1)

	return events[0].GetWorkflowExecutionStartedEventAttributes().GetInput()
}

// A response whose payloads name no namespace is bound to the one its request
// names by ID.
func TestAdminEncryptorBindsRepliesToTheRequestNamespaceID(t *testing.T) {
	names := &fakeNamespaces{names: map[string]string{"id-a": "a"}}
	e := requireAdminEncryptor(t, namespacedConfig(reverseConfig(realVault(t, crypto.BindingPermissive)), names))

	req := &adminservice.GetWorkflowExecutionRawHistoryV2Request{NamespaceId: "id-a"}
	reply := &adminservice.GetWorkflowExecutionRawHistoryV2Response{
		HistoryBatches: []*common.DataBlob{eventBlob(t, payloads("1"))},
	}
	require.NoError(t, adminCall(t, e.Unary, req, reply, func() error { return nil }))
	require.Equal(t, []string{"peer-a"}, boundTo(inputInBlob(t, reply.GetHistoryBatches()[0])))
}

// A request naming its namespace by name has had the local name stamped on the
// context by the server, so it is bound by that with no lookup.
func TestAdminEncryptorBindsToTheStampedNamespace(t *testing.T) {
	names := &fakeNamespaces{}
	e := requireAdminEncryptor(t, namespacedConfig(adminConfig(realVault(t, crypto.BindingPermissive)), names))

	ctx := context.WithValue(t.Context(), NamespaceKey, "a")
	req := &adminservice.ImportWorkflowExecutionRequest{
		Namespace:      "a",
		HistoryBatches: []*common.DataBlob{eventBlob(t, payloads("1"))},
	}
	require.NoError(t, adminCallWith(ctx, e.Unary, req, new(adminservice.ImportWorkflowExecutionResponse), func() error { return nil }))
	require.Equal(t, []string{"peer-a"}, boundTo(inputInBlob(t, req.GetHistoryBatches()[0])))
	require.Zero(t, names.calls, "no lookup")
}

// A stream proxy passes on what the encryptor failed, and only that, so a
// failed visit is marked, with a status that keeps the detail out of what
// reaches the peer.
func TestAdminEncryptorStreamVisitErrors(t *testing.T) {
	t.Run("a failed visit is a VisitError with a generic status", func(t *testing.T) {
		e := requireAdminEncryptor(t, reverseConfig(&fakeVault{}))
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: io.EOF,
			recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{streamMessages(hsmTask("x"))}})

		err := cs.RecvMsg(new(adminservice.StreamWorkflowReplicationMessagesResponse))

		var visitErr *VisitError
		require.ErrorAs(t, err, &visitErr)
		require.ErrorContains(t, err, "StateMachineNode", "the detail stays in the error for logging")

		st := status.Convert(err)
		require.Equal(t, codes.Internal, st.Code(), "not the cause's FailedPrecondition")
		require.NotContains(t, st.Message(), "StateMachineNode", "the status carries no detail")
		require.Contains(t, st.Message(), "see its logs")
	})

	t.Run("an unresolved namespace is Internal and not named", func(t *testing.T) {
		cfg := namespacedConfig(reverseConfig(realVault(t, crypto.BindingStrict)), &fakeNamespaces{err: errors.New("frontend unavailable")})
		cfg.StrictNamespaces = true
		e := requireAdminEncryptor(t, cfg)
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: io.EOF,
			recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{streamMessages(activityTaskIn("id-a", payloads("1")))}})

		err := cs.RecvMsg(new(adminservice.StreamWorkflowReplicationMessagesResponse))
		require.ErrorContains(t, err, "id-a")

		st := status.Convert(err)
		require.Equal(t, codes.Internal, st.Code())
		require.NotContains(t, st.Message(), "id-a")
	})

	t.Run("a stream error whose details fail to open", func(t *testing.T) {
		e := requireAdminEncryptor(t, adminConfig(&fakeVault{openErr: errors.New("kms down")}))
		st, err := status.New(codes.FailedPrecondition, "nope").WithDetails(&errordetails.QueryFailedFailure{
			Failure: &failure.Failure{FailureInfo: &failure.Failure_ApplicationFailureInfo{
				ApplicationFailureInfo: &failure.ApplicationFailureInfo{Details: sealed(t, "detail")},
			}},
		})
		require.NoError(t, err)
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: st.Err()})

		requireGenericVisitError(t, cs.RecvMsg(new(adminservice.StreamWorkflowReplicationMessagesResponse)), "kms down")
	})

	t.Run("errors from the stream itself are left as they are and not reported", func(t *testing.T) {
		obs := &recordingVisitErrorObserver{}
		cfg := adminConfig(&fakeVault{})
		cfg.VisitErrors = obs
		e := requireAdminEncryptor(t, cfg)
		upstream := status.Error(codes.Unavailable, "remote went away")
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: upstream})

		err := cs.RecvMsg(new(adminservice.StreamWorkflowReplicationMessagesResponse))

		var visitErr *VisitError
		require.NotErrorAs(t, err, &visitErr)
		require.Equal(t, "remote went away", status.Convert(err).Message())
		require.Empty(t, obs.streams)
	})

	t.Run("a failed visit is reported as a stream, not a unary call", func(t *testing.T) {
		obs := &recordingVisitErrorObserver{}
		cfg := reverseConfig(&fakeVault{})
		cfg.VisitErrors = obs
		e := requireAdminEncryptor(t, cfg)
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), err: io.EOF,
			recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{streamMessages(hsmTask("x"))}})

		require.Error(t, cs.RecvMsg(new(adminservice.StreamWorkflowReplicationMessagesResponse)))
		require.Empty(t, obs.failed)
		require.Len(t, obs.streams, 1)
		require.Contains(t, obs.streams[0], "/StreamWorkflowReplicationMessages: ")
		require.Contains(t, obs.streams[0], "StateMachineNode")
	})
}

// recordingVisitErrorObserver remembers every failed visit it is told about,
// as "method: error", unary calls and streams apart.
type recordingVisitErrorObserver struct {
	mu      sync.Mutex
	failed  []string
	streams []string
}

func (o *recordingVisitErrorObserver) VisitFailed(method string, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.failed = append(o.failed, method+": "+err.Error())
}

func (o *recordingVisitErrorObserver) StreamVisitFailed(method string, err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.streams = append(o.streams, method+": "+err.Error())
}

// requireGenericVisitError checks err is a VisitError as gRPC will see it:
// outermost, Internal, and saying nothing about the cause.
func requireGenericVisitError(t *testing.T, err error, cause string) {
	t.Helper()

	visitErr, ok := err.(*VisitError) //nolint:errorlint // outermost is the point: gRPC rewrites the message of a wrapped status
	require.True(t, ok, "want a *VisitError outermost, got %T: %v", err, err)
	require.ErrorContains(t, visitErr, cause, "the cause stays in the error")

	st := status.Convert(err)
	require.Equal(t, codes.Internal, st.Code())
	require.NotContains(t, st.Message(), cause)
}

const testAdminMethod = "/temporal.server.api.adminservice.v1.AdminService/Test"

func TestAdminEncryptorUnaryVisitErrors(t *testing.T) {
	t.Run("a request that fails to seal", func(t *testing.T) {
		obs := &recordingVisitErrorObserver{}
		cfg := AdminEncryptorConfig{EncryptorConfig: EncryptorConfig{Enabled: true, Vault: &fakeVault{sealErr: errors.New("kms down")}}}
		cfg.VisitErrors = obs
		e := requireAdminEncryptor(t, cfg)

		req := &adminservice.ImportWorkflowExecutionRequest{HistoryBatches: []*common.DataBlob{eventBlob(t, payloads("x"))}}
		err := adminCall(t, e.Unary, req, new(adminservice.ImportWorkflowExecutionResponse), func() error { return nil })

		requireGenericVisitError(t, err, "kms down")
		require.Len(t, obs.failed, 1)
		require.Contains(t, obs.failed[0], testAdminMethod+": ")
		require.Contains(t, obs.failed[0], "kms down")
	})

	t.Run("a reply that fails to open", func(t *testing.T) {
		obs := &recordingVisitErrorObserver{}
		cfg := adminConfig(&fakeVault{openErr: errors.New("kms down")})
		cfg.VisitErrors = obs
		e := requireAdminEncryptor(t, cfg)

		reply := new(adminservice.GetDLQReplicationMessagesResponse)
		err := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, reply, func() error {
			reply.ReplicationTasks = []*replication.ReplicationTask{activityTask(sealed(t, "x"))}
			return nil
		})

		requireGenericVisitError(t, err, "kms down")
		require.Len(t, obs.failed, 1)
	})

	t.Run("error details that fail to open", func(t *testing.T) {
		obs := &recordingVisitErrorObserver{}
		cfg := adminConfig(&fakeVault{openErr: errors.New("kms down")})
		cfg.VisitErrors = obs
		e := requireAdminEncryptor(t, cfg)

		st, err := status.New(codes.FailedPrecondition, "nope").WithDetails(&errordetails.QueryFailedFailure{
			Failure: &failure.Failure{FailureInfo: &failure.Failure_ApplicationFailureInfo{
				ApplicationFailureInfo: &failure.ApplicationFailureInfo{Details: sealed(t, "detail")},
			}},
		})
		require.NoError(t, err)

		got := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, new(adminservice.ReapplyEventsResponse), st.Err)

		requireGenericVisitError(t, got, "kms down")
		require.Len(t, obs.failed, 1)
	})

	t.Run("errors from the call itself are left as they are and not reported", func(t *testing.T) {
		obs := &recordingVisitErrorObserver{}
		cfg := adminConfig(&fakeVault{})
		cfg.VisitErrors = obs
		e := requireAdminEncryptor(t, cfg)

		upstream := status.Error(codes.Unavailable, "remote went away")
		got := adminCall(t, e.Unary, &adminservice.ReapplyEventsRequest{}, new(adminservice.ReapplyEventsResponse), func() error {
			return upstream
		})

		require.Equal(t, codes.Unavailable, status.Code(got))
		require.Equal(t, "remote went away", status.Convert(got).Message())
		require.Empty(t, obs.failed)
	})
}

// Nothing wraps a visit failure today, but if something did, gRPC would take
// the wrapper's text for the status, cause and all.
func TestSurfaceUnwrapsAVisitError(t *testing.T) {
	cause := &VisitError{err: errors.New("kms down for key arn:secret")}
	wrapped := fmt.Errorf("visiting: %w", cause)
	require.Contains(t, status.Convert(wrapped).Message(), "arn:secret", "control: gRPC uses the wrapper's text")

	requireGenericVisitError(t, surface(wrapped), "arn:secret")

	plain := errors.New("not ours")
	require.Same(t, plain, surface(plain))
}
