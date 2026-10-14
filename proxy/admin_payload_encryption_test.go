package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/codec"
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	apiproxy "go.temporal.io/api/proxy"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	persistencespb "go.temporal.io/server/api/persistence/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/persistence/serialization"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/logging"
)

const adminPayloadCanary = `"admin-payload-canary"`

// fixedAdminPayloadService returns a payload-bearing AdminService response.
// The client calls this through the proxy directly; no replication loop or
// second Temporal cluster participates in the assertion.
type fixedAdminPayloadService struct {
	adminservice.UnimplementedAdminServiceServer

	payload     string
	historyBlob *commonpb.DataBlob
}

func (s *fixedAdminPayloadService) DescribeMutableState(
	context.Context,
	*adminservice.DescribeMutableStateRequest,
) (*adminservice.DescribeMutableStateResponse, error) {
	return &adminservice.DescribeMutableStateResponse{DatabaseMutableState: mutableStateWithPayload(s.payload)}, nil
}

func (s *fixedAdminPayloadService) GetWorkflowExecutionRawHistory(
	context.Context,
	*adminservice.GetWorkflowExecutionRawHistoryRequest,
) (*adminservice.GetWorkflowExecutionRawHistoryResponse, error) {
	return &adminservice.GetWorkflowExecutionRawHistoryResponse{HistoryBatches: []*commonpb.DataBlob{s.historyBlob}}, nil
}

func (s *fixedAdminPayloadService) GetWorkflowExecutionRawHistoryV2(
	context.Context,
	*adminservice.GetWorkflowExecutionRawHistoryV2Request,
) (*adminservice.GetWorkflowExecutionRawHistoryV2Response, error) {
	return &adminservice.GetWorkflowExecutionRawHistoryV2Response{HistoryBatches: []*commonpb.DataBlob{s.historyBlob}}, nil
}

func (s *fixedAdminPayloadService) SyncWorkflowState(
	context.Context,
	*adminservice.SyncWorkflowStateRequest,
) (*adminservice.SyncWorkflowStateResponse, error) {
	return &adminservice.SyncWorkflowStateResponse{
		VersionedTransitionArtifact: &replicationspb.VersionedTransitionArtifact{
			StateAttributes: &replicationspb.VersionedTransitionArtifact_SyncWorkflowStateSnapshotAttributes{
				SyncWorkflowStateSnapshotAttributes: &replicationspb.SyncWorkflowStateSnapshotAttributes{
					State: mutableStateWithPayload(s.payload),
				},
			},
		},
	}, nil
}

func (s *fixedAdminPayloadService) GetDLQReplicationMessages(
	context.Context,
	*adminservice.GetDLQReplicationMessagesRequest,
) (*adminservice.GetDLQReplicationMessagesResponse, error) {
	return &adminservice.GetDLQReplicationMessagesResponse{
		ReplicationTasks: []*replicationspb.ReplicationTask{{
			Attributes: &replicationspb.ReplicationTask_SyncActivityTaskAttributes{
				SyncActivityTaskAttributes: &replicationspb.SyncActivityTaskAttributes{
					Details: &commonpb.Payloads{Payloads: []*commonpb.Payload{plainPayload(s.payload)}},
				},
			},
		}},
	}, nil
}

func mutableStateWithPayload(payload string) *persistencespb.WorkflowMutableState {
	return &persistencespb.WorkflowMutableState{
		ExecutionInfo: &persistencespb.WorkflowExecutionInfo{
			Memo: map[string]*commonpb.Payload{"canary": plainPayload(payload)},
		},
	}
}

func startAdminPayloadService(t *testing.T, address, payload string) {
	t.Helper()

	listener, err := net.Listen("tcp", address)
	require.NoError(t, err)

	historyBlob, err := serialization.NewSerializer().SerializeEvents([]*historypb.HistoryEvent{{
		EventId:   1,
		EventType: enumspb.EVENT_TYPE_WORKFLOW_EXECUTION_STARTED,
		Attributes: &historypb.HistoryEvent_WorkflowExecutionStartedEventAttributes{
			WorkflowExecutionStartedEventAttributes: &historypb.WorkflowExecutionStartedEventAttributes{
				Input: &commonpb.Payloads{Payloads: []*commonpb.Payload{plainPayload(payload)}},
			},
		},
	}})
	require.NoError(t, err)

	server := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(server, &fixedAdminPayloadService{
		payload:     payload,
		historyBlob: historyBlob,
	})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
}

func assertAdminResponseSealed(t *testing.T, response proto.Message) {
	t.Helper()

	wire, err := proto.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(wire), adminPayloadCanary)

	visited := 0
	err = interceptor.VisitAdminPayloads(t.Context(), response, interceptor.AdminVisitOptions{
		Payloads: &apiproxy.VisitPayloadsOptions{
			SkipSearchAttributes: true,
			Visitor: func(
				_ *apiproxy.VisitPayloadsContext,
				payloads []*commonpb.Payload,
			) ([]*commonpb.Payload, error) {
				for _, payload := range payloads {
					visited++
					require.Equal(t, codec.EncryptionEncoding, encodingOf(payload))
					require.NotContains(t, string(payload.GetData()), adminPayloadCanary)
				}
				return payloads, nil
			},
		},
		Opaque: func(string, []byte) error { return nil },
	})
	require.NoError(t, err)
	require.Positive(t, visited, "response contained no inspectable payloads")
}

func TestAdminClientsReceiveSealedPayloadsFromProxy(t *testing.T) {
	ports := getDynamicPorts(t, 4)
	localTemporal, remoteTemporal, proxyOutbound, proxyInbound := ports[0], ports[1], ports[2], ports[3]

	startAdminPayloadService(t, localTemporal, adminPayloadCanary)
	startAdminPayloadService(t, remoteTemporal, `"unused"`)

	cfg := makeTCPClusterConfig(
		"admin-payload-encryption",
		localFVI,
		remoteFVI,
		proxyOutbound,
		localTemporal,
		proxyInbound,
		proxyOutbound,
		remoteTemporal,
	)
	cfg.EncryptionConfig = config.EncryptionConfig{
		Enabled: true,
		Default: &config.KeyPolicy{URI: testEncryptionKeyURI, Duration: time.Hour},
	}

	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	connection, err := NewClusterConnection(t.Context(), cfg, nil, loggers)
	require.NoError(t, err)
	connection.Start()

	conn, err := grpc.NewClient(proxyInbound, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := adminservice.NewAdminServiceClient(conn)
	tests := map[string]func() (proto.Message, error){
		"DescribeMutableState": func() (proto.Message, error) {
			return client.DescribeMutableState(t.Context(), &adminservice.DescribeMutableStateRequest{})
		},
		"GetWorkflowExecutionRawHistory": func() (proto.Message, error) {
			return client.GetWorkflowExecutionRawHistory(t.Context(), &adminservice.GetWorkflowExecutionRawHistoryRequest{})
		},
		"GetWorkflowExecutionRawHistoryV2": func() (proto.Message, error) {
			return client.GetWorkflowExecutionRawHistoryV2(t.Context(), &adminservice.GetWorkflowExecutionRawHistoryV2Request{})
		},
		"SyncWorkflowState": func() (proto.Message, error) {
			return client.SyncWorkflowState(t.Context(), &adminservice.SyncWorkflowStateRequest{})
		},
		"GetDLQReplicationMessages": func() (proto.Message, error) {
			return client.GetDLQReplicationMessages(t.Context(), &adminservice.GetDLQReplicationMessagesRequest{})
		},
	}

	for name, call := range tests {
		t.Run(name, func(t *testing.T) {
			response, err := call()
			require.NoError(t, err)
			assertAdminResponseSealed(t, response)
		})
	}
}
