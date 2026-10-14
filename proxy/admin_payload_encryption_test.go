package proxy

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/codec"
	commonpb "go.temporal.io/api/common/v1"
	adminservice "go.temporal.io/server/api/adminservice/v1"
	replicationspb "go.temporal.io/server/api/replication/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/logging"
)

const adminPayloadCanary = `"admin-payload-canary"`

// fixedAdminPayloadService returns a payload-bearing AdminService response.
// The client calls this through the proxy directly; no replication loop or
// second Temporal cluster participates in the assertion.
type fixedAdminPayloadService struct {
	adminservice.UnimplementedAdminServiceServer

	payload string
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

func startAdminPayloadService(t *testing.T, address, payload string) {
	t.Helper()

	listener, err := net.Listen("tcp", address)
	require.NoError(t, err)

	server := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(server, &fixedAdminPayloadService{payload: payload})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
}

func TestAdminClientReceivesSealedPayloadFromProxy(t *testing.T) {
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

	response, err := adminservice.NewAdminServiceClient(conn).GetDLQReplicationMessages(
		t.Context(),
		&adminservice.GetDLQReplicationMessagesRequest{},
	)
	require.NoError(t, err)

	tasks := response.GetReplicationTasks()
	require.Len(t, tasks, 1)
	require.NotNil(t, tasks[0].GetSyncActivityTaskAttributes())
	payloads := tasks[0].GetSyncActivityTaskAttributes().GetDetails().GetPayloads()
	require.Len(t, payloads, 1)
	require.Equal(t, codec.EncryptionEncoding, encodingOf(payloads[0]))
	require.NotContains(t, string(payloads[0].GetData()), adminPayloadCanary)
}
