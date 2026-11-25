package interceptor

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/sdk/v1"
	"go.temporal.io/proxy/pkg/crypto"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/persistence/v1"
	replication "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/chasm"
	"go.temporal.io/server/chasm/lib/nexusoperation/gen/nexusoperationpb/v1"
	"go.temporal.io/server/chasm/lib/scheduler/gen/schedulerpb/v1"
	"go.temporal.io/server/chasm/lib/workflow/gen/workflowpb/v1"
	"go.temporal.io/server/common/persistence/serialization"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/anypb"
)

// TestChasmSchemaMessagesResolve checks that the embedded descriptors build and
// that every message the schema names resolves, so no CHASM state falls back to
// opaque for want of a descriptor.
func TestChasmSchemaMessagesResolve(t *testing.T) {
	_, err := chasmFiles()
	require.NoError(t, err)

	var names []protoreflect.FullName
	for _, c := range chasmComponents {
		if c.state != "" {
			names = append(names, c.state)
		}
		for _, ch := range c.children {
			if ch.data != "" {
				names = append(names, ch.data)
			}
		}
	}
	for _, task := range chasmTasks {
		if task.data != "" {
			names = append(names, task.data)
		}
	}
	require.NotEmpty(t, names)

	for _, name := range names {
		md, err := chasmMessage(name)
		require.NoError(t, err)
		require.Equal(t, name, md.FullName())
	}
}

// TestDecodeChasmPathMatchesServer checks the proxy's copy of the server's node
// path decoding against the server's own, on escapes and collection items.
func TestDecodeChasmPathMatchesServer(t *testing.T) {
	keys := []string{
		"",
		"Visibility",
		"Visibility$SA",
		"Operations#op-1",
		"Operations$op-1$Outcome",
		`Backfillers#a\$b`,
		`Backfillers$a\#b\\c$EventLog`,
		"Callbacks#\\!x",
		`a\\`,
	}
	for _, key := range keys {
		want, err := chasm.DefaultPathEncoder.Decode(key)
		require.NoError(t, err, key)

		got, err := decodeChasmPath(key)
		require.NoError(t, err, key)
		require.Equal(t, len(want), len(got), key)
		for i := range want {
			require.Equal(t, want[i], got[i], key)
		}
	}

	_, err := decodeChasmPath(`trailing\`)
	require.ErrorContains(t, err, "ends with an escape character")
}

// TestChasmDataType checks the schema walk from an archetype, the root
// component of an execution, to the message a data node holds.
func TestChasmDataType(t *testing.T) {
	var (
		activityID  = chasm.GenerateTypeID("activity.activity")
		nexusOpID   = chasm.GenerateTypeID("nexusoperation.operation")
		schedulerID = chasm.GenerateTypeID("scheduler.scheduler")
		workflowID  = chasm.GenerateTypeID("workflow.workflow")
	)

	tests := []struct {
		name string
		root uint32
		path []string
		want protoreflect.FullName
	}{
		{"nexus request", nexusOpID, []string{"RequestData"}, "temporal.server.chasm.lib.nexusoperation.proto.v1.OperationRequestData"},
		{"activity request shares the name", activityID, []string{"RequestData"}, "temporal.server.chasm.lib.activity.proto.v1.ActivityRequestData"},
		{"nexus outcome under a workflow", workflowID, []string{"Operations", "op-1", "Outcome"}, "temporal.server.chasm.lib.nexusoperation.proto.v1.OperationOutcome"},
		{"scheduler search attributes", schedulerID, []string{"Visibility", "SA"}, "temporal.api.common.v1.SearchAttributes"},
		{"scheduler last completion", schedulerID, []string{"LastCompletionResult"}, "temporal.server.chasm.lib.scheduler.proto.v1.LastCompletionResult"},
		{"collection node", workflowID, []string{"Operations"}, ""},
		{"component node", schedulerID, []string{"Generator"}, ""},
		{"root", schedulerID, nil, ""},
		{"unknown name", schedulerID, []string{"Nope"}, ""},
		{"below a data node", nexusOpID, []string{"RequestData", "x"}, ""},
		{"unknown root", 1, []string{"RequestData"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := chasmDataType(tt.root, tt.path)
			require.Equal(t, tt.want != "", ok)
			require.Equal(t, tt.want, got)
		})
	}
}

// componentNode returns a CHASM component node of the type fqn names.
func componentNode(fqn string) *persistence.ChasmNode {
	return &persistence.ChasmNode{Metadata: &persistence.ChasmNodeMetadata{
		Attributes: &persistence.ChasmNodeMetadata_ComponentAttributes{
			ComponentAttributes: &persistence.ChasmComponentAttributes{TypeId: chasm.GenerateTypeID(fqn)},
		},
	}}
}

// TestChasmTreeDataTypeWithoutArchetype checks how a data node is typed when
// the message names no archetype: from the nearest component above it.
func TestChasmTreeDataTypeWithoutArchetype(t *testing.T) {
	const outcome protoreflect.FullName = "temporal.server.chasm.lib.nexusoperation.proto.v1.OperationOutcome"
	path := []string{"Operations", "op-1", "Outcome"}

	tests := []struct {
		name      string
		nodes     map[string]*persistence.ChasmNode
		archetype uint32
		want      protoreflect.FullName
	}{
		{
			name:  "parent operation present",
			nodes: map[string]*persistence.ChasmNode{"Operations#op-1": componentNode("nexusoperation.operation")},
			want:  outcome,
		},
		{
			name:  "only the workflow root present",
			nodes: map[string]*persistence.ChasmNode{"": componentNode("workflow.workflow")},
			want:  outcome,
		},
		{
			name:  "no component above",
			nodes: map[string]*persistence.ChasmNode{},
		},
		{
			// An activity has no Operations, so the archetype walk fails and the
			// operation above the node types it.
			name:      "archetype that cannot reach the node",
			nodes:     map[string]*persistence.ChasmNode{"Operations#op-1": componentNode("nexusoperation.operation")},
			archetype: chasm.GenerateTypeID("activity.activity"),
			want:      outcome,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tree, err := newChasmTree(tt.nodes)
			require.NoError(t, err)

			got, ok := tree.dataType(tt.archetype, path)
			require.Equal(t, tt.want != "", ok)
			require.Equal(t, tt.want, got)
		})
	}
}

// encodeChasm serializes m the way the server's CHASM tree does.
func encodeChasm(t *testing.T, m proto.Message) *common.DataBlob {
	t.Helper()
	blob, err := serialization.Encode(m, serialization.WithDeterministicProto3)
	require.NoError(t, err)
	return blob
}

// refuseOpaque fails the test when anything is reported as opaque.
func refuseOpaque(t *testing.T) func(string, []byte) error {
	return func(path string, _ []byte) error {
		t.Errorf("unexpected opaque data at %s", path)
		return nil
	}
}

func TestVisitChasmBlobSealsNexusRequest(t *testing.T) {
	blob := encodeChasm(t, &nexusoperationpb.OperationRequestData{
		Input:       &common.Payload{Data: []byte("input")},
		NexusHeader: map[string]string{"k": "v"},
		UserMetadata: &sdk.UserMetadata{
			Summary: &common.Payload{Data: []byte("summary")},
			Details: &common.Payload{Data: []byte("details")},
		},
	})

	var calls atomic.Int32
	opts := sealOpts(&calls)
	opts.Opaque = refuseOpaque(t)
	require.NoError(t, visitChasmBlob(context.Background(), opts, "test", "temporal.server.chasm.lib.nexusoperation.proto.v1.OperationRequestData", blob))
	require.Equal(t, int32(3), calls.Load())

	var got nexusoperationpb.OperationRequestData
	require.NoError(t, serialization.Decode(blob, &got))
	require.Equal(t, "sealed:input", string(got.GetInput().GetData()))
	require.Equal(t, "sealed:summary", string(got.GetUserMetadata().GetSummary().GetData()))
	require.Equal(t, "sealed:details", string(got.GetUserMetadata().GetDetails().GetData()))
	require.Equal(t, map[string]string{"k": "v"}, got.GetNexusHeader())
}

func TestVisitChasmBlobNexusOperationParentData(t *testing.T) {
	const state = "temporal.server.chasm.lib.nexusoperation.proto.v1.OperationState"
	parent := &workflowpb.NexusOperationParentData{ScheduledEventId: 5, ScheduledEventToken: []byte{1, 2, 3}}
	failureWithDetails := &failure.Failure{
		Message: "boom",
		FailureInfo: &failure.Failure_ApplicationFailureInfo{ApplicationFailureInfo: &failure.ApplicationFailureInfo{
			Details: &common.Payloads{Payloads: []*common.Payload{{Data: []byte("details")}}},
		}},
	}

	t.Run("known parent data", func(t *testing.T) {
		packed, err := anypb.New(parent)
		require.NoError(t, err)
		blob := encodeChasm(t, &nexusoperationpb.OperationState{ParentData: packed, LastAttemptFailure: failureWithDetails})

		var calls atomic.Int32
		opts := sealOpts(&calls)
		opts.Opaque = refuseOpaque(t)
		require.NoError(t, visitChasmBlob(context.Background(), opts, "test", state, blob))
		require.Equal(t, int32(1), calls.Load())

		var got nexusoperationpb.OperationState
		require.NoError(t, serialization.Decode(blob, &got))
		var gotParent workflowpb.NexusOperationParentData
		require.NoError(t, got.GetParentData().UnmarshalTo(&gotParent))
		require.True(t, proto.Equal(parent, &gotParent))
		require.Equal(t, "sealed:details", string(got.GetLastAttemptFailure().GetApplicationFailureInfo().GetDetails().GetPayloads()[0].GetData()))
	})

	t.Run("unknown parent data", func(t *testing.T) {
		packed, err := anypb.New(&common.Payload{Data: []byte("secret")})
		require.NoError(t, err)
		blob := encodeChasm(t, &nexusoperationpb.OperationState{ParentData: packed})

		var calls atomic.Int32
		var opaque []string
		opts := sealOpts(&calls)
		opts.Opaque = func(path string, _ []byte) error {
			opaque = append(opaque, path)
			return nil
		}
		require.NoError(t, visitChasmBlob(context.Background(), opts, "test", state, blob))
		require.Len(t, opaque, 1)
		require.True(t, strings.HasSuffix(opaque[0], "/parent_data"), opaque[0])
	})
}

func TestVisitChasmBlobFailsClosed(t *testing.T) {
	const request = "temporal.server.chasm.lib.nexusoperation.proto.v1.OperationRequestData"
	collect := func(opaque *[]string) func(string, []byte) error {
		return func(path string, _ []byte) error {
			*opaque = append(*opaque, path)
			return nil
		}
	}

	t.Run("undecodable", func(t *testing.T) {
		blob := &common.DataBlob{EncodingType: enums.ENCODING_TYPE_PROTO3, Data: []byte{0xff}}
		var calls atomic.Int32
		var opaque []string
		opts := sealOpts(&calls)
		opts.Opaque = collect(&opaque)
		require.NoError(t, visitChasmBlob(context.Background(), opts, "test", request, blob))
		require.Equal(t, []string{"test"}, opaque)
		require.Equal(t, []byte{0xff}, blob.GetData())
	})

	t.Run("json", func(t *testing.T) {
		data := []byte(`{"input":{"data":"c2VjcmV0"}}`)
		blob := &common.DataBlob{EncodingType: enums.ENCODING_TYPE_JSON, Data: data}
		var calls atomic.Int32
		var opaque []string
		opts := sealOpts(&calls)
		opts.Opaque = collect(&opaque)
		require.NoError(t, visitChasmBlob(context.Background(), opts, "test", request, blob))
		require.Equal(t, []string{"test"}, opaque)
		require.Equal(t, data, blob.GetData())
		require.Zero(t, calls.Load())
	})

	t.Run("unknown fields", func(t *testing.T) {
		blob := encodeChasm(t, &nexusoperationpb.OperationRequestData{Input: &common.Payload{Data: []byte("input")}})
		blob.Data = protowire.AppendVarint(protowire.AppendTag(blob.Data, 999, protowire.VarintType), 7)

		var calls atomic.Int32
		var opaque []string
		opts := sealOpts(&calls)
		opts.Opaque = collect(&opaque)
		require.NoError(t, visitChasmBlob(context.Background(), opts, "test", request, blob))
		require.Equal(t, []string{"test/<unknown fields>"}, opaque)

		var got nexusoperationpb.OperationRequestData
		require.NoError(t, serialization.Decode(blob, &got))
		require.Equal(t, "sealed:input", string(got.GetInput().GetData()))
		require.NotEmpty(t, got.ProtoReflect().GetUnknown(), "unknown field dropped")
	})
}

func TestVisitChasmBlobSearchAttributes(t *testing.T) {
	const migration = "temporal.server.chasm.lib.scheduler.proto.v1.SchedulerMigrationState"
	for _, skip := range []bool{true, false} {
		t.Run(fmt.Sprintf("skip=%v", skip), func(t *testing.T) {
			wantSA := "sa"
			if !skip {
				wantSA = "sealed:sa"
			}
			var calls atomic.Int32
			opts := sealOpts(&calls)
			opts.PayloadOptions.SkipSearchAttributes = skip
			opts.Opaque = refuseOpaque(t)

			// A bare map of payloads named search_attributes, beside a memo.
			blob := encodeChasm(t, &schedulerpb.SchedulerMigrationState{
				SearchAttributes: map[string]*common.Payload{"CustomKeywordField": {Data: []byte("sa")}},
				Memo:             map[string]*common.Payload{"note": {Data: []byte("memo")}},
			})
			require.NoError(t, visitChasmBlob(context.Background(), opts, "test", migration, blob))
			var gotState schedulerpb.SchedulerMigrationState
			require.NoError(t, serialization.Decode(blob, &gotState))
			require.Equal(t, wantSA, string(gotState.GetSearchAttributes()["CustomKeywordField"].GetData()))
			require.Equal(t, "sealed:memo", string(gotState.GetMemo()["note"].GetData()))

			// A Visibility SA data node, which is a SearchAttributes message.
			blob = encodeChasm(t, &common.SearchAttributes{IndexedFields: map[string]*common.Payload{"CustomKeywordField": {Data: []byte("sa")}}})
			require.NoError(t, visitChasmBlob(context.Background(), opts, "test", "temporal.api.common.v1.SearchAttributes", blob))
			var gotSA common.SearchAttributes
			require.NoError(t, serialization.Decode(blob, &gotSA))
			require.Equal(t, wantSA, string(gotSA.GetIndexedFields()["CustomKeywordField"].GetData()))
		})
	}
}

func TestVisitChasmNodes(t *testing.T) {
	task := func(fqn string, m proto.Message) *persistence.ChasmComponentAttributes_Task {
		return &persistence.ChasmComponentAttributes_Task{TypeId: chasm.GenerateTypeID(fqn), Data: encodeChasm(t, m)}
	}
	dataNode := func(m proto.Message) *persistence.ChasmNode {
		return &persistence.ChasmNode{
			Metadata: &persistence.ChasmNodeMetadata{Attributes: &persistence.ChasmNodeMetadata_DataAttributes{DataAttributes: &persistence.ChasmDataAttributes{}}},
			Data:     encodeChasm(t, m),
		}
	}

	root := componentNode("nexusoperation.operation")
	root.Data = encodeChasm(t, &nexusoperationpb.OperationState{LastAttemptFailure: &failure.Failure{
		FailureInfo: &failure.Failure_ApplicationFailureInfo{ApplicationFailureInfo: &failure.ApplicationFailureInfo{
			Details: &common.Payloads{Payloads: []*common.Payload{{Data: []byte("details")}}},
		}},
	}})
	attrs := root.GetMetadata().GetComponentAttributes()
	attrs.UserMetadata = &sdk.UserMetadata{Summary: &common.Payload{Data: []byte("summary")}}
	attrs.SideEffectTasks = []*persistence.ChasmComponentAttributes_Task{
		task("nexusoperation.invocation", &nexusoperationpb.InvocationTask{Attempt: 2}),
		{TypeId: 1, Data: &common.DataBlob{EncodingType: enums.ENCODING_TYPE_PROTO3, Data: []byte("unknown task")}},
	}

	mystery := componentNode("nope.nope")
	mystery.Data = &common.DataBlob{EncodingType: enums.ENCODING_TYPE_PROTO3, Data: []byte("unknown component")}

	nodes := map[string]*persistence.ChasmNode{
		"":              root,
		"RequestData":   dataNode(&nexusoperationpb.OperationRequestData{Input: &common.Payload{Data: []byte("input")}}),
		"Visibility":    componentNode("core.vis"),
		"Visibility$SA": dataNode(&common.SearchAttributes{IndexedFields: map[string]*common.Payload{"k": {Data: []byte("sa")}}}),
		"Mystery":       mystery,
		"Nope":          dataNode(&common.Payload{Data: []byte("untyped")}),
	}

	var calls atomic.Int32
	var opaque []string
	opts := sealOpts(&calls)
	opts.PayloadOptions.SkipSearchAttributes = false
	opts.Opaque = func(path string, _ []byte) error {
		opaque = append(opaque, path)
		return nil
	}
	ctx := withArchetypeID(context.Background(), chasm.GenerateTypeID("nexusoperation.operation"))
	require.NoError(t, visitChasmNodes(ctx, opts, "ChasmNodes", nodes))

	require.Equal(t, int32(4), calls.Load())
	require.ElementsMatch(t, []string{
		"ChasmNodes[]/SideEffectTasks[1]/Data",
		"ChasmNodes[Mystery]/Data",
		"ChasmNodes[Nope]/Data",
	}, opaque)

	var request nexusoperationpb.OperationRequestData
	require.NoError(t, serialization.Decode(nodes["RequestData"].GetData(), &request))
	require.Equal(t, "sealed:input", string(request.GetInput().GetData()))
	require.Equal(t, "sealed:summary", string(attrs.GetUserMetadata().GetSummary().GetData()))
	require.Equal(t, []byte("unknown component"), mystery.GetData().GetData())
}

// A unary reply can carry a CHASM mutation whose data node has no component
// above it in the message. The request names the archetype, which types it.
func TestAdminEncryptorSealsChasmMutationInUnaryReply(t *testing.T) {
	vault := realVault(t, crypto.BindingOff)
	req := &adminservice.SyncWorkflowStateRequest{
		NamespaceId: "ns-id",
		ArchetypeId: chasm.GenerateTypeID("nexusoperation.operation"),
	}
	reply := &adminservice.SyncWorkflowStateResponse{
		VersionedTransitionArtifact: &replication.VersionedTransitionArtifact{
			StateAttributes: &replication.VersionedTransitionArtifact_SyncWorkflowStateMutationAttributes{
				SyncWorkflowStateMutationAttributes: &replication.SyncWorkflowStateMutationAttributes{
					StateMutation: &persistence.WorkflowMutableStateMutation{
						UpdatedChasmNodes: map[string]*persistence.ChasmNode{
							"Outcome": {
								Metadata: &persistence.ChasmNodeMetadata{Attributes: &persistence.ChasmNodeMetadata_DataAttributes{DataAttributes: &persistence.ChasmDataAttributes{}}},
								Data: encodeChasm(t, &nexusoperationpb.OperationOutcome{Variant: &nexusoperationpb.OperationOutcome_Successful_{
									Successful: &nexusoperationpb.OperationOutcome_Successful{Result: &common.Payload{Data: []byte("secret")}},
								}}),
							},
						},
					},
				},
			},
		},
	}
	result := func() *common.Payload {
		node := reply.GetVersionedTransitionArtifact().GetSyncWorkflowStateMutationAttributes().GetStateMutation().GetUpdatedChasmNodes()["Outcome"]
		var outcome nexusoperationpb.OperationOutcome
		require.NoError(t, serialization.Decode(node.GetData(), &outcome))
		return outcome.GetSuccessful().GetResult()
	}

	local := requireAdminEncryptor(t, reverseConfig(vault))
	require.NoError(t, adminCall(t, local.Unary, req, reply, func() error { return nil }))
	require.NotContains(t, string(result().GetData()), "secret", "sealed on the way to the peer")

	peer := requireAdminEncryptor(t, adminConfig(vault))
	require.NoError(t, adminCall(t, peer.Unary, req, reply, func() error { return nil }))
	require.Equal(t, "secret", string(result().GetData()), "opened on the way back")
}

// chasmMutationTask is a replication task carrying a CHASM mutation of an
// execution of the given archetype, as stream replication sends it.
func chasmMutationTask(archetype uint32, nodes map[string]*persistence.ChasmNode) *replication.ReplicationTask {
	return &replication.ReplicationTask{
		Attributes: &replication.ReplicationTask_SyncVersionedTransitionTaskAttributes{
			SyncVersionedTransitionTaskAttributes: &replication.SyncVersionedTransitionTaskAttributes{
				NamespaceId: "ns-id",
				ArchetypeId: archetype,
				VersionedTransitionArtifact: &replication.VersionedTransitionArtifact{
					StateAttributes: &replication.VersionedTransitionArtifact_SyncWorkflowStateMutationAttributes{
						SyncWorkflowStateMutationAttributes: &replication.SyncWorkflowStateMutationAttributes{
							StateMutation: &persistence.WorkflowMutableStateMutation{UpdatedChasmNodes: nodes},
						},
					},
				},
			},
		},
	}
}

// The customer case: a standalone Nexus operation replicating over the stream,
// with allowOpaquePlaintext off. Known CHASM state is sealed and the stream
// carries on; state of an unknown type still fails it.
func TestAdminEncryptorStreamSealsChasmState(t *testing.T) {
	nexusOp := chasm.GenerateTypeID("nexusoperation.operation")
	dataNode := func(m proto.Message) *persistence.ChasmNode {
		return &persistence.ChasmNode{
			Metadata: &persistence.ChasmNodeMetadata{Attributes: &persistence.ChasmNodeMetadata_DataAttributes{DataAttributes: &persistence.ChasmDataAttributes{}}},
			Data:     encodeChasm(t, m),
		}
	}
	nodes := func(r *adminservice.StreamWorkflowReplicationMessagesResponse) map[string]*persistence.ChasmNode {
		return r.GetMessages().GetReplicationTasks()[0].GetSyncVersionedTransitionTaskAttributes().
			GetVersionedTransitionArtifact().GetSyncWorkflowStateMutationAttributes().GetStateMutation().GetUpdatedChasmNodes()
	}

	t.Run("known state is sealed", func(t *testing.T) {
		root := componentNode("nexusoperation.operation")
		root.GetMetadata().GetComponentAttributes().SideEffectTasks = []*persistence.ChasmComponentAttributes_Task{{
			TypeId: chasm.GenerateTypeID("nexusoperation.invocation"),
			Data:   encodeChasm(t, &nexusoperationpb.InvocationTask{Attempt: 3}),
		}}
		msg := streamMessages(chasmMutationTask(nexusOp, map[string]*persistence.ChasmNode{
			"":            root,
			"RequestData": dataNode(&nexusoperationpb.OperationRequestData{Input: &common.Payload{Data: []byte("secret input")}}),
			"Outcome": dataNode(&nexusoperationpb.OperationOutcome{Variant: &nexusoperationpb.OperationOutcome_Successful_{
				Successful: &nexusoperationpb.OperationOutcome_Successful{Result: &common.Payload{Data: []byte("secret result")}},
			}}),
		}))

		e := requireAdminEncryptor(t, reverseConfig(realVault(t, crypto.BindingOff)))
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{msg}})
		got := new(adminservice.StreamWorkflowReplicationMessagesResponse)
		require.NoError(t, cs.RecvMsg(got))

		var request nexusoperationpb.OperationRequestData
		require.NoError(t, serialization.Decode(nodes(got)["RequestData"].GetData(), &request))
		require.NotContains(t, string(request.GetInput().GetData()), "secret")

		var outcome nexusoperationpb.OperationOutcome
		require.NoError(t, serialization.Decode(nodes(got)["Outcome"].GetData(), &outcome))
		require.NotContains(t, string(outcome.GetSuccessful().GetResult().GetData()), "secret")

		var task nexusoperationpb.InvocationTask
		require.NoError(t, serialization.Decode(nodes(got)[""].GetMetadata().GetComponentAttributes().GetSideEffectTasks()[0].GetData(), &task))
		require.EqualValues(t, 3, task.GetAttempt())
	})

	t.Run("unknown state still fails the stream", func(t *testing.T) {
		mystery := componentNode("nope.nope")
		mystery.Data = &common.DataBlob{EncodingType: enums.ENCODING_TYPE_PROTO3, Data: []byte("secret")}
		msg := streamMessages(chasmMutationTask(nexusOp, map[string]*persistence.ChasmNode{"Mystery": mystery}))

		e := requireAdminEncryptor(t, reverseConfig(realVault(t, crypto.BindingOff)))
		cs := openStream(t, e, &fakeClientStream{ctx: t.Context(), recv: []*adminservice.StreamWorkflowReplicationMessagesResponse{msg}})
		err := cs.RecvMsg(new(adminservice.StreamWorkflowReplicationMessagesResponse))
		require.ErrorContains(t, err, "cannot seal opaque chasm data")
	})
}
