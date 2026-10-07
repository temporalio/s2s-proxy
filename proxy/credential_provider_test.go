package proxy

import (
	"context"
	"net"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/logging"
)

const testBearerToken = "Bearer local-token"

type staticCredentials struct {
	requireTLS bool
}

func (staticCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": testBearerToken}, nil
}

func (c staticCredentials) RequireTransportSecurity() bool { return c.requireTLS }

type staticCredentialProvider struct {
	creds credentials.PerRPCCredentials
}

func (p staticCredentialProvider) Get() credentials.PerRPCCredentials { return p.creds }

// authRecordingServer is a fake Temporal server that records the authorization header of every call it receives,
// for any service, and answers Unimplemented.
type authRecordingServer struct {
	mu      sync.Mutex
	headers map[string][]string // method -> authorization values
}

func startAuthRecordingServer(t *testing.T, address string) *authRecordingServer {
	s := &authRecordingServer{headers: make(map[string][]string)}
	listener, err := net.Listen("tcp", address)
	require.NoError(t, err)
	server := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		method, _ := grpc.MethodFromServerStream(stream)
		md, _ := metadata.FromIncomingContext(stream.Context())
		s.mu.Lock()
		s.headers[method] = md.Get("authorization")
		s.mu.Unlock()
		return status.Error(codes.Unimplemented, "recorded")
	}))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return s
}

func (s *authRecordingServer) authorization(method string) ([]string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	values, ok := s.headers[method]
	return values, ok
}

func newCredentialTestConnection(
	t *testing.T,
	a plccAddresses,
	credentialsEnabled bool,
	provider auth.CredentialProvider,
) *ClusterConnection {
	connConfig := makeTCPClusterConfig("creds", localFVI, remoteFVI, "",
		a.localTemporalAddr, a.localProxyOutbound, a.localProxyInbound, a.remoteTemporalAddr)
	connConfig.Local.Credentials = &config.CredentialsConfig{Enabled: credentialsEnabled}
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	cc, err := newClusterConnection(t.Context(), connConfig, loggers, provider)
	require.NoError(t, err)
	return cc
}

func TestCredentialsAreAttachedToLocalCallsOnly(t *testing.T) {
	a := getDynamicPlccAddresses(t)
	localTemporal := startAuthRecordingServer(t, a.localTemporalAddr)
	remoteTemporal := startAuthRecordingServer(t, a.remoteTemporalAddr)
	cc := newCredentialTestConnection(t, a, true, staticCredentialProvider{creds: staticCredentials{}})

	// Every service, unary and streaming, made on the local client carries the token.
	ctx := t.Context()
	_, _ = adminservice.NewAdminServiceClient(cc.inboundClient).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	_, _ = workflowservice.NewWorkflowServiceClient(cc.inboundClient).GetSystemInfo(ctx, &workflowservice.GetSystemInfoRequest{})
	_, _ = operatorservice.NewOperatorServiceClient(cc.inboundClient).ListClusters(ctx, &operatorservice.ListClustersRequest{})
	stream, err := adminservice.NewAdminServiceClient(cc.inboundClient).StreamWorkflowReplicationMessages(ctx)
	require.NoError(t, err)
	_, _ = stream.Recv()

	for _, method := range []string{
		"/temporal.server.api.adminservice.v1.AdminService/DescribeCluster",
		"/temporal.api.workflowservice.v1.WorkflowService/GetSystemInfo",
		"/temporal.api.operatorservice.v1.OperatorService/ListClusters",
		"/temporal.server.api.adminservice.v1.AdminService/StreamWorkflowReplicationMessages",
	} {
		values, ok := localTemporal.authorization(method)
		require.True(t, ok, "local server never saw %s", method)
		require.Equal(t, []string{testBearerToken}, values, method)
	}

	// The remote side never sees the local token.
	_, _ = adminservice.NewAdminServiceClient(cc.outboundClient).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	values, ok := remoteTemporal.authorization("/temporal.server.api.adminservice.v1.AdminService/DescribeCluster")
	require.True(t, ok)
	require.Empty(t, values)
}

func TestCredentialsAreNotAttachedUnlessEnabled(t *testing.T) {
	a := getDynamicPlccAddresses(t)
	localTemporal := startAuthRecordingServer(t, a.localTemporalAddr)
	startAuthRecordingServer(t, a.remoteTemporalAddr)
	cc := newCredentialTestConnection(t, a, false, staticCredentialProvider{creds: staticCredentials{}})

	_, _ = adminservice.NewAdminServiceClient(cc.inboundClient).DescribeCluster(t.Context(), &adminservice.DescribeClusterRequest{})
	values, ok := localTemporal.authorization("/temporal.server.api.adminservice.v1.AdminService/DescribeCluster")
	require.True(t, ok)
	require.Empty(t, values)
}

func TestCreateClientRejectsUnusableCredentials(t *testing.T) {
	enabled := &config.CredentialsConfig{Enabled: true}
	tcp := func(tls encryption.TLSConfig) config.ClusterDefinition {
		return config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpClient:      config.TCPTLSInfo{ConnectionString: "localhost:7233", TLSConfig: tls},
			Credentials:    enabled,
		}
	}
	tests := []struct {
		name      string
		cluster   config.ClusterDefinition
		provider  auth.CredentialProvider
		wantError string
	}{
		{
			name:      "credentials enabled without a provider",
			cluster:   tcp(encryption.TLSConfig{}),
			provider:  auth.EmptyCredentialProvider{},
			wantError: "credentials are enabled but no CredentialProvider is configured",
		},
		{
			name:      "provider returns no credentials",
			cluster:   tcp(encryption.TLSConfig{}),
			provider:  staticCredentialProvider{},
			wantError: "credential provider returned no credentials",
		},
		{
			name:      "mux connection",
			cluster:   config.ClusterDefinition{ConnectionType: config.ConnTypeMuxClient, Credentials: enabled},
			provider:  staticCredentialProvider{creds: staticCredentials{}},
			wantError: "credentials require a tcp connection",
		},
		{
			name:      "plaintext connection with credentials that require TLS",
			cluster:   tcp(encryption.TLSConfig{}),
			provider:  staticCredentialProvider{creds: staticCredentials{requireTLS: true}},
			wantError: "credentials require TLS",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := createClient(t.Context(), "test", tc.cluster, "inbound", tc.provider)
			require.ErrorContains(t, err, tc.wantError)
		})
	}

	t.Run("empty provider leaves a mux connection alone", func(t *testing.T) {
		_, err := createClient(t.Context(), "test", config.ClusterDefinition{ConnectionType: config.ConnTypeMuxClient},
			"inbound", auth.EmptyCredentialProvider{})
		require.NoError(t, err)
	})
}
