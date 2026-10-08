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

const (
	testBearerToken = "Bearer local-token"
	peerBearerToken = "Bearer peer-token"

	describeClusterMethod = "/temporal.server.api.adminservice.v1.AdminService/DescribeCluster"
)

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

// recordedCredentials are the credential headers one call arrived with.
type recordedCredentials struct {
	authorization       []string
	authorizationExtras []string
}

// authRecordingServer is a fake Temporal server that records the credential headers of every call it receives, for
// any service, and answers Unimplemented.
type authRecordingServer struct {
	mu      sync.Mutex
	headers map[string]recordedCredentials // method -> credential headers
}

func startAuthRecordingServer(t *testing.T, address string) *authRecordingServer {
	s := &authRecordingServer{headers: make(map[string]recordedCredentials)}
	listener, err := net.Listen("tcp", address)
	require.NoError(t, err)
	server := grpc.NewServer(grpc.UnknownServiceHandler(func(_ any, stream grpc.ServerStream) error {
		method, _ := grpc.MethodFromServerStream(stream)
		md, _ := metadata.FromIncomingContext(stream.Context())
		s.mu.Lock()
		s.headers[method] = recordedCredentials{
			authorization:       md.Get("authorization"),
			authorizationExtras: md.Get("authorization-extras"),
		}
		s.mu.Unlock()
		return status.Error(codes.Unimplemented, "recorded")
	}))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return s
}

func (s *authRecordingServer) credentials(t *testing.T, method string) recordedCredentials {
	s.mu.Lock()
	defer s.mu.Unlock()
	recorded, ok := s.headers[method]
	require.True(t, ok, "server never saw %s", method)
	return recorded
}

func newCredentialTestConnection(
	t *testing.T,
	a plccAddresses,
	identity config.CredentialIdentity,
	provider auth.CredentialProvider,
) *ClusterConnection {
	connConfig := makeTCPClusterConfig("creds", localFVI, remoteFVI, "",
		a.localTemporalAddr, a.localProxyOutbound, a.localProxyInbound, a.remoteTemporalAddr)
	connConfig.Local.Credentials = &config.CredentialsConfig{Identity: identity}
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	cc, err := newClusterConnection(t.Context(), connConfig, loggers, provider)
	require.NoError(t, err)
	return cc
}

// forwardedContext carries the credential headers a peer sent, the way the replication stream forwarder passes them on.
func forwardedContext(t *testing.T) context.Context {
	return metadata.NewOutgoingContext(t.Context(), metadata.Pairs(
		"authorization", peerBearerToken,
		"authorization-extras", "peer-extras",
	))
}

func TestProxyIdentityReplacesForwardedCredentials(t *testing.T) {
	a := getDynamicPlccAddresses(t)
	localTemporal := startAuthRecordingServer(t, a.localTemporalAddr)
	remoteTemporal := startAuthRecordingServer(t, a.remoteTemporalAddr)
	cc := newCredentialTestConnection(t, a, config.CredentialIdentityProxy, staticCredentialProvider{creds: staticCredentials{}})

	// Every service, unary and streaming, carries only the proxy's token, whatever the peer sent.
	ctx := forwardedContext(t)
	_, _ = adminservice.NewAdminServiceClient(cc.inboundClient).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	_, _ = workflowservice.NewWorkflowServiceClient(cc.inboundClient).GetSystemInfo(ctx, &workflowservice.GetSystemInfoRequest{})
	_, _ = operatorservice.NewOperatorServiceClient(cc.inboundClient).ListClusters(ctx, &operatorservice.ListClustersRequest{})
	stream, err := adminservice.NewAdminServiceClient(cc.inboundClient).StreamWorkflowReplicationMessages(ctx)
	require.NoError(t, err)
	_, _ = stream.Recv()

	for _, method := range []string{
		describeClusterMethod,
		"/temporal.api.workflowservice.v1.WorkflowService/GetSystemInfo",
		"/temporal.api.operatorservice.v1.OperatorService/ListClusters",
		"/temporal.server.api.adminservice.v1.AdminService/StreamWorkflowReplicationMessages",
	} {
		recorded := localTemporal.credentials(t, method)
		require.Equal(t, []string{testBearerToken}, recorded.authorization, method)
		require.Empty(t, recorded.authorizationExtras, method)
	}

	// The remote side never sees the proxy's token.
	_, _ = adminservice.NewAdminServiceClient(cc.outboundClient).DescribeCluster(t.Context(), &adminservice.DescribeClusterRequest{})
	require.Empty(t, remoteTemporal.credentials(t, describeClusterMethod).authorization)
}

func TestStripIdentityStripsForwardedCredentials(t *testing.T) {
	a := getDynamicPlccAddresses(t)
	localTemporal := startAuthRecordingServer(t, a.localTemporalAddr)
	startAuthRecordingServer(t, a.remoteTemporalAddr)
	// A provider is configured but unused: identity "strip" never sends credentials.
	cc := newCredentialTestConnection(t, a, config.CredentialIdentityStrip, staticCredentialProvider{creds: staticCredentials{}})

	_, _ = adminservice.NewAdminServiceClient(cc.inboundClient).DescribeCluster(forwardedContext(t), &adminservice.DescribeClusterRequest{})
	recorded := localTemporal.credentials(t, describeClusterMethod)
	require.Empty(t, recorded.authorization)
	require.Empty(t, recorded.authorizationExtras)
}

func TestDefaultIdentityForwardsCredentials(t *testing.T) {
	for _, identity := range []config.CredentialIdentity{"", config.CredentialIdentityDefault} {
		t.Run("identity="+string(identity), func(t *testing.T) {
			a := getDynamicPlccAddresses(t)
			localTemporal := startAuthRecordingServer(t, a.localTemporalAddr)
			startAuthRecordingServer(t, a.remoteTemporalAddr)
			cc := newCredentialTestConnection(t, a, identity, staticCredentialProvider{creds: staticCredentials{}})

			_, _ = adminservice.NewAdminServiceClient(cc.inboundClient).DescribeCluster(forwardedContext(t), &adminservice.DescribeClusterRequest{})
			recorded := localTemporal.credentials(t, describeClusterMethod)
			require.Equal(t, []string{peerBearerToken}, recorded.authorization)
			require.Equal(t, []string{"peer-extras"}, recorded.authorizationExtras)
		})
	}
}

func TestCreateClientRejectsUnusableCredentials(t *testing.T) {
	proxyIdentity := &config.CredentialsConfig{Identity: config.CredentialIdentityProxy}
	tcp := func(tls encryption.TLSConfig) config.ClusterDefinition {
		return config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpClient:      config.TCPTLSInfo{ConnectionString: "localhost:7233", TLSConfig: tls},
			Credentials:    proxyIdentity,
		}
	}
	tests := []struct {
		name      string
		cluster   config.ClusterDefinition
		provider  auth.CredentialProvider
		wantError string
	}{
		{
			name:      "proxy identity without a provider",
			cluster:   tcp(encryption.TLSConfig{}),
			provider:  auth.EmptyCredentialProvider{},
			wantError: `credentials identity "proxy" but no CredentialProvider is configured`,
		},
		{
			name:      "provider returns no credentials",
			cluster:   tcp(encryption.TLSConfig{}),
			provider:  staticCredentialProvider{},
			wantError: "credential provider returned no credentials",
		},
		{
			name:      "mux connection",
			cluster:   config.ClusterDefinition{ConnectionType: config.ConnTypeMuxClient, Credentials: proxyIdentity},
			provider:  staticCredentialProvider{creds: staticCredentials{}},
			wantError: "credentials require a tcp connection",
		},
		{
			name:      "plaintext connection with credentials that require TLS",
			cluster:   tcp(encryption.TLSConfig{}),
			provider:  staticCredentialProvider{creds: staticCredentials{requireTLS: true}},
			wantError: "credentials require TLS",
		},
		{
			name: "unsupported identity",
			cluster: config.ClusterDefinition{
				ConnectionType: config.ConnTypeTCP,
				Credentials:    &config.CredentialsConfig{Identity: "everyone"},
			},
			provider:  auth.EmptyCredentialProvider{},
			wantError: `unsupported credentials identity "everyone"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := createClient(t.Context(), "test", tc.cluster, "inbound", tc.provider)
			require.ErrorContains(t, err, tc.wantError)
		})
	}

	t.Run("default identity leaves a mux connection alone", func(t *testing.T) {
		_, err := createClient(t.Context(), "test", config.ClusterDefinition{ConnectionType: config.ConnTypeMuxClient},
			"inbound", auth.EmptyCredentialProvider{})
		require.NoError(t, err)
	})

	t.Run("strip identity works on a mux connection", func(t *testing.T) {
		_, err := createClient(t.Context(), "test", config.ClusterDefinition{
			ConnectionType: config.ConnTypeMuxClient,
			Credentials:    &config.CredentialsConfig{Identity: config.CredentialIdentityStrip},
		}, "inbound", auth.EmptyCredentialProvider{})
		require.NoError(t, err)
	})
}
