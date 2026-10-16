package interceptor

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/errordetails/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/history/v1"
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
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
		require.ErrorContains(t, err, "allowOpaquePlaintext")
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
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
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
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
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
		require.Equal(t, codes.FailedPrecondition, status.Code(err))
	})
}
