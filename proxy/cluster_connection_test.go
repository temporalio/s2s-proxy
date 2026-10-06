package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/codec"
	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/endtoendtest/testservices"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

const (
	localFVI  = 10
	remoteFVI = 20
)

func init() {
	_ = os.Setenv("TEMPORAL_TEST_LOG_LEVEL", "error")
	mux.MuxManagerStartDelay = 0
}

func getDynamicPorts(t *testing.T, num int) []string {
	listeners := make([]net.Listener, num)
	output := make([]string, num)
	for i := range num {
		var err error
		listeners[i], err = net.Listen("tcp", "localhost:0")
		require.NoError(t, err)
		output[i] = listeners[i].Addr().String()
	}
	// Ensure the listeners aren't sitting on the ports
	for _, l := range listeners {
		_ = l.Close()
	}
	t.Log("Prepared ports:", output)
	return output
}

type pairedLocalClusterConnection struct {
	localTemporal        *testservices.TemporalServerWithListen
	cancelLocalTemporal  context.CancelFunc
	remoteTemporal       *testservices.TemporalServerWithListen
	cancelRemoteTemporal context.CancelFunc
	localCC              *ClusterConnection
	cancelLocalCC        context.CancelFunc
	remoteCC             *ClusterConnection
	cancelRemoteCC       context.CancelFunc
	clientFromLocal      *grpc.ClientConn
	clientFromRemote     *grpc.ClientConn
	addresses            plccAddresses
}
type plccAddresses struct {
	localTemporalAddr   string
	remoteTemporalAddr  string
	localProxyOutbound  string
	remoteProxyInbound  string
	localProxyInbound   string
	remoteProxyOutbound string
}

func getDynamicPlccAddresses(t *testing.T) plccAddresses {
	a := plccAddresses{}
	addresses := getDynamicPorts(t, 6)
	// Server listening ports can be visualized like this:
	//  0          1                          2       3                             4        5
	// local, ->outbound | local proxy | <-inbound, inbound-> | remote proxy | <-outbound, remote
	// Outbound traffic goes from 0->1->3->5
	// Inbound traffic goes from 5->4->2->0
	a.localTemporalAddr, a.remoteTemporalAddr = addresses[0], addresses[5]
	a.localProxyOutbound, a.remoteProxyInbound = addresses[1], addresses[3]
	a.localProxyInbound, a.remoteProxyOutbound = addresses[2], addresses[4]
	return a
}

func (plcc *pairedLocalClusterConnection) StartAll(t *testing.T) {
	plcc.localTemporal.Start()
	var localCtx context.Context
	localCtx, plcc.cancelLocalTemporal = context.WithCancel(t.Context())
	context.AfterFunc(localCtx, plcc.localTemporal.Stop)
	plcc.remoteTemporal.Start()
	var remoteCtx context.Context
	remoteCtx, plcc.cancelRemoteTemporal = context.WithCancel(t.Context())
	context.AfterFunc(remoteCtx, plcc.remoteTemporal.Stop)
	plcc.remoteCC.Start()
	plcc.localCC.Start()
}

func makeTCPClusterConfig(name string, localFvi int64, remoteFvi int64, replicationEndpoint string,
	localServer string, localToRemoteServer string, remoteToLocalServer string, remoteServer string,
) config.ClusterConnConfig {
	return config.ClusterConnConfig{
		Name: name,
		Local: config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpServer: config.TCPTLSInfo{
				ConnectionString: remoteToLocalServer,
			},
			TcpClient: config.TCPTLSInfo{
				ConnectionString: localServer,
			},
		},
		Remote: config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpServer: config.TCPTLSInfo{
				ConnectionString: localToRemoteServer,
			},
			TcpClient: config.TCPTLSInfo{
				ConnectionString: remoteServer,
			},
		},
		FVITranslation: config.IntMapping{
			Local:  localFvi,
			Remote: remoteFvi,
		},
		ReplicationEndpoint: replicationEndpoint,
	}
}

func makeMuxClusterConfig(name string, client config.ConnectionType,
	localFVI int64, remoteFVI int64, replicationEndpoint string, localTemporal string, outboundServer string, muxAddr string,
	edits ...func(connConfig *config.ClusterConnConfig),
) config.ClusterConnConfig {
	cc := config.ClusterConnConfig{
		Name:                name,
		ReplicationEndpoint: replicationEndpoint,
		FVITranslation: config.IntMapping{
			Local:  localFVI,
			Remote: remoteFVI,
		},
		Local: config.ClusterDefinition{
			ConnectionType: config.ConnTypeTCP,
			TcpServer: config.TCPTLSInfo{
				ConnectionString: outboundServer,
			},
			TcpClient: config.TCPTLSInfo{
				ConnectionString: localTemporal,
			},
		},
		Remote: config.ClusterDefinition{
			ConnectionType: client,
			MuxAddressInfo: config.TCPTLSInfo{
				ConnectionString: muxAddr,
				// No TLS
			},
		},
	}
	for _, f := range edits {
		f(&cc)
	}
	return cc
}

func makeEchoServer(name string, listenAddress string, logger log.Logger) *testservices.TemporalServerWithListen {
	logger.Info("Starting echo server", tag.NewStringTag("name", name), tag.Address(listenAddress))
	return testservices.NewTemporalAPIServer(name,
		testservices.NewEchoAdminService(name, nil, logger),
		testservices.NewEchoWorkflowService(name, logger),
		nil, listenAddress, logger)
}

func newPairedLocalClusterConnection(t *testing.T, isMux bool, loggers logging.LoggerProvider) *pairedLocalClusterConnection {
	a := getDynamicPlccAddresses(t)

	localTemporal := makeEchoServer("local", a.localTemporalAddr, loggers.Get("root"))
	remoteTemporal := makeEchoServer("remote", a.remoteTemporalAddr, loggers.Get("root"))

	var localCC, remoteCC *ClusterConnection
	var cancelLocalCC, cancelRemoteCC context.CancelFunc
	var err error
	if !isMux {
		var localCtx context.Context
		localCtx, cancelLocalCC = context.WithCancel(t.Context())
		localCC, err = NewClusterConnection(localCtx, makeTCPClusterConfig("TCP-only Connection Local Proxy",
			localFVI, remoteFVI, a.localProxyOutbound, a.localTemporalAddr, a.localProxyInbound, a.localProxyOutbound, a.remoteProxyInbound), nil, loggers)
		require.NoError(t, err)

		var remoteCtx context.Context
		remoteCtx, cancelRemoteCC = context.WithCancel(t.Context())
		remoteCC, err = NewClusterConnection(remoteCtx, makeTCPClusterConfig("TCP-only Connection Remote Proxy",
			remoteFVI, localFVI, a.remoteProxyOutbound, a.remoteTemporalAddr, a.remoteProxyInbound, a.remoteProxyOutbound, a.localProxyInbound), nil, loggers)
		require.NoError(t, err)
	} else {
		var localCtx context.Context
		localCtx, cancelLocalCC = context.WithCancel(t.Context())
		localCC, err = NewClusterConnection(localCtx, makeMuxClusterConfig("Mux Connection Local Establishing Proxy",
			config.ConnTypeMuxClient, localFVI, remoteFVI, a.localProxyOutbound, a.localTemporalAddr, a.localProxyOutbound, a.remoteProxyInbound), nil, loggers)
		require.NoError(t, err)

		var remoteCtx context.Context
		remoteCtx, cancelRemoteCC = context.WithCancel(t.Context())
		remoteCC, err = NewClusterConnection(remoteCtx, makeMuxClusterConfig("Mux Connection Remote Receiving Proxy",
			config.ConnTypeMuxServer, remoteFVI, localFVI, a.remoteProxyOutbound, a.remoteTemporalAddr, a.remoteProxyOutbound, a.remoteProxyInbound), nil, loggers)
		require.NoError(t, err)
	}
	clientFromLocal, err := grpc.NewClient(a.localProxyOutbound, grpcutil.MakeDialOptions(nil, metrics.GetStandardGRPCClientInterceptor("outbound-local"))...)
	require.NoError(t, err)
	clientFromRemote, err := grpc.NewClient(a.remoteProxyOutbound, grpcutil.MakeDialOptions(nil, metrics.GetStandardGRPCClientInterceptor("outbound-remote"))...)
	require.NoError(t, err)
	return &pairedLocalClusterConnection{
		localTemporal:    localTemporal,
		remoteTemporal:   remoteTemporal,
		localCC:          localCC,
		cancelLocalCC:    cancelLocalCC,
		remoteCC:         remoteCC,
		cancelRemoteCC:   cancelRemoteCC,
		clientFromLocal:  clientFromLocal,
		clientFromRemote: clientFromRemote,
		addresses:        a,
	}
}

// runNamespaceChain sends req through the namespace interceptors a server in the
// given direction installs, and reports what StampNamespace left on the context
// along with the namespace the handler finally saw. The second value is what
// tells the two orderings apart: both directions must stamp the local name, but
// only one of them hands the handler a rewritten request.
func runNamespaceChain(
	t *testing.T,
	callerNamesLocalNamespaces bool,
	req *workflowservice.StartWorkflowExecutionRequest,
) (stamped string, handlerSaw string) {
	t.Helper()

	// Mirrors NewClusterConnection: the outbound server is handed the
	// local-to-remote map and the inbound server its inverse.
	translation := config.StringTranslator{
		Mappings: []config.StringMapping{{Local: "local-ns", Remote: "remote-ns"}},
	}
	nsTranslations, err := translation.AsLocalToRemoteBiMap()
	require.NoError(t, err)
	if !callerNamesLocalNamespaces {
		nsTranslations = nsTranslations.Inverse()
	}

	logger := log.NewNoopLogger()
	translate := interceptor.NewTranslationInterceptor(logger, []interceptor.Translator{
		interceptor.NewNamespaceNameTranslator(logger,
			nsTranslations.AsMap(), nsTranslations.Inverse().AsMap()),
	})

	info := &grpc.UnaryServerInfo{FullMethod: api.WorkflowServicePrefix + "StartWorkflowExecution"}
	handler := grpc.UnaryHandler(func(ctx context.Context, r any) (any, error) {
		stamped, _ = ctx.Value(interceptor.NamespaceKey).(string)
		handlerSaw = r.(*workflowservice.StartWorkflowExecutionRequest).GetNamespace()

		return &workflowservice.StartWorkflowExecutionResponse{}, nil
	})

	chain := stampAndTranslate(callerNamesLocalNamespaces, translate)
	for i := len(chain) - 1; i >= 0; i-- {
		next, intercept := handler, chain[i]
		handler = func(ctx context.Context, r any) (any, error) {
			return intercept(ctx, r, info, next)
		}
	}

	_, err = handler(t.Context(), req)
	require.NoError(t, err)

	return stamped, handlerSaw
}

func TestTCPClusterConnection(t *testing.T) {
	loggerProvider := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	plcc := newPairedLocalClusterConnection(t, false, loggerProvider)
	plcc.StartAll(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	resp, err := adminservice.NewAdminServiceClient(plcc.clientFromLocal).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoErrorf(t, err, "Got error from remote server. Configs:\nlocal %s\nremote %s", plcc.localCC.Describe(), plcc.remoteCC.Describe())
	require.Equal(t, int64(remoteFVI), resp.FailoverVersionIncrement, "Should see remote FVI from the local outbound")
	require.Equal(t, "remote-EchoAdminService", resp.ClusterName, "Should see remote EchoAdminService from the local outbound")
	cancel()
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	resp, err = adminservice.NewAdminServiceClient(plcc.clientFromRemote).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoErrorf(t, err, "Got error from remote server. Configs:\nlocal %s\nremote %s", plcc.localCC.Describe(), plcc.remoteCC.Describe())
	require.Equal(t, int64(localFVI), resp.FailoverVersionIncrement, "Should see local FVI from the remote outbound")
	require.Equal(t, "local-EchoAdminService", resp.ClusterName, "Should see local EchoAdminService from the remote outbound")
	cancel()
}

func TestMuxClusterConnection(t *testing.T) {
	loggerProvider := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	plcc := newPairedLocalClusterConnection(t, true, loggerProvider)
	plcc.StartAll(t)
	t.Log("Started plcc")

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	resp, err := adminservice.NewAdminServiceClient(plcc.clientFromLocal).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoErrorf(t, err, "Got error from remote server. Configs:\nlocal %s\nremote %s", plcc.localCC.Describe(), plcc.remoteCC.Describe())
	require.Equal(t, int64(remoteFVI), resp.FailoverVersionIncrement, "Should see remote FVI from the local outbound")
	require.Equal(t, "remote-EchoAdminService", resp.ClusterName, "Should see remote EchoAdminService from the local outbound")
	t.Log("Called remote!")
	cancel()
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	resp, err = adminservice.NewAdminServiceClient(plcc.clientFromRemote).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoErrorf(t, err, "Got error from remote server. Configs:\nlocal %s\nremote %s", plcc.localCC.Describe(), plcc.remoteCC.Describe())
	require.Equal(t, int64(localFVI), resp.FailoverVersionIncrement, "Should see local FVI from the remote outbound")
	require.Equal(t, "local-EchoAdminService", resp.ClusterName, "Should see local EchoAdminService from the remote outbound")
	t.Log("Finished!")
	cancel()
}

func TestMuxCCFailover(t *testing.T) {
	loggerProvider := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	plcc := newPairedLocalClusterConnection(t, true, loggerProvider)
	plcc.StartAll(t)

	plcc.cancelRemoteCC()
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	_, err := adminservice.NewAdminServiceClient(plcc.clientFromRemote).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.Error(t, err)
	cancel()
	newConnection, err := NewClusterConnection(t.Context(),
		makeMuxClusterConfig("newRemoteMux", config.ConnTypeMuxServer, remoteFVI, localFVI, plcc.addresses.remoteProxyOutbound,
			plcc.addresses.remoteTemporalAddr, plcc.addresses.remoteProxyOutbound, plcc.addresses.remoteProxyInbound,
			func(cc *config.ClusterConnConfig) { cc.Remote.MuxCount = 5 }), nil, loggerProvider)
	require.NoError(t, err)
	newConnection.Start()
	// Wait for localCC's client retry...
	timeout := time.Now().Add(2 * time.Second)
	var resp *adminservice.DescribeClusterResponse
	err = errors.New("didn't complete a single request")
	for time.Now().Before(timeout) {
		ctx, cancel = context.WithTimeout(t.Context(), time.Second)
		resp, err = adminservice.NewAdminServiceClient(plcc.clientFromRemote).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
		if err == nil {
			break
		}
	}
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "local-EchoAdminService", resp.ClusterName, "Local cluster connection should have reconnected")
	cancel()
	ctx, cancel = context.WithTimeout(t.Context(), time.Second)
	resp, err = adminservice.NewAdminServiceClient(plcc.clientFromLocal).DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
	require.NoError(t, err)
	require.Equal(t, "remote-EchoAdminService", resp.ClusterName, "Local cluster connection should have reconnected")
	cancel()
}

func TestNewProxyReportsConfigurationErrors(t *testing.T) {
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))

	t.Run("no cluster connections", func(t *testing.T) {
		_, err := NewProxy(config.NewMockConfigProvider(config.S2SProxyConfig{}), loggers)
		require.EqualError(t, err, "cannot create proxy: clusterConnections is empty")
	})

	t.Run("a cluster connection that cannot be built", func(t *testing.T) {
		a := getDynamicPlccAddresses(t)
		broken := makeTCPClusterConfig("broken", localFVI, remoteFVI, a.localProxyOutbound,
			a.localTemporalAddr, a.localProxyInbound, a.localProxyOutbound, a.remoteProxyInbound)
		broken.Local.ConnectionType = "not-a-connection-type"

		_, err := NewProxy(config.NewMockConfigProvider(config.S2SProxyConfig{
			ClusterConnections: []config.ClusterConnConfig{broken},
		}), loggers)
		require.Error(t, err)
		require.Contains(t, err.Error(), `cannot create cluster connection "broken"`)
	})

	t.Run("an invalid encryption config", func(t *testing.T) {
		a := getDynamicPlccAddresses(t)
		cc := makeTCPClusterConfig("encrypted", localFVI, remoteFVI, a.localProxyOutbound,
			a.localTemporalAddr, a.localProxyInbound, a.localProxyOutbound, a.remoteProxyInbound)
		cc.EncryptionConfig = config.EncryptionConfig{
			Enabled: true,
			Default: &config.KeyPolicy{URI: "vault://typo", Duration: time.Hour},
		}

		// Config is checked before anything is built, so a bad key URI costs no
		// listeners and the error names the field that caused it.
		_, err := NewProxy(config.NewMockConfigProvider(config.S2SProxyConfig{
			ClusterConnections: []config.ClusterConnConfig{cc},
		}), loggers)
		require.ErrorContains(t, err, "cannot create proxy: invalid config: ")
		require.ErrorContains(t, err, "clusterConnections[0].encryption.default: uri: invalid key URI: vault://typo")
	})
}

func TestNewProxyRejectsDuplicateClusterConnectionNames(t *testing.T) {
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	a := getDynamicPlccAddresses(t)
	first := makeTCPClusterConfig("same", localFVI, remoteFVI, a.localProxyOutbound,
		a.localTemporalAddr, a.localProxyInbound, a.localProxyOutbound, a.remoteProxyInbound)
	second := makeTCPClusterConfig("same", remoteFVI, localFVI, a.remoteProxyOutbound,
		a.remoteTemporalAddr, a.remoteProxyInbound, a.remoteProxyOutbound, a.localProxyInbound)

	_, err := NewProxy(config.NewMockConfigProvider(config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{first, second},
	}), loggers)
	require.EqualError(t, err, `cannot create proxy: duplicate cluster connection name "same"`)
}

func TestTCPListenerIsReleasedWhenNeverStarted(t *testing.T) {
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	a := getDynamicPlccAddresses(t)

	ctx, cancel := context.WithCancel(t.Context())
	cc, err := NewClusterConnection(ctx, makeTCPClusterConfig("abandoned", localFVI, remoteFVI,
		a.localProxyOutbound, a.localTemporalAddr, a.localProxyInbound, a.localProxyOutbound,
		a.remoteProxyInbound), nil, loggers)
	require.NoError(t, err)

	cancel() // never Started

	// A TCP connection builds an inbound and an outbound server, and both bind a socket.
	for _, address := range []string{a.localProxyInbound, a.localProxyOutbound} {
		require.Eventually(t, func() bool {
			l, err := net.Listen("tcp", address)
			if err != nil {
				return false
			}
			_ = l.Close()
			return true
		}, 5*time.Second, 50*time.Millisecond, "listener on %s was never released", address)
	}
	runtime.KeepAlive(cc)
}

func TestProxyStampAndTranslateNamespace(t *testing.T) {
	t.Run("outbound stamps the name the local cluster sent, before translation", func(t *testing.T) {
		stamped, handlerSaw := runNamespaceChain(t, true,
			&workflowservice.StartWorkflowExecutionRequest{Namespace: "local-ns"})

		require.Equal(t, "local-ns", stamped)
		require.Equal(t, "remote-ns", handlerSaw,
			"the translator still has to rewrite the request on its way to the peer")
	})

	t.Run("inbound stamps the local name, after translation", func(t *testing.T) {
		stamped, handlerSaw := runNamespaceChain(t, false,
			&workflowservice.StartWorkflowExecutionRequest{Namespace: "remote-ns"})

		require.Equal(t, "local-ns", stamped,
			"stamping the peer's name would miss the encryption config's per-namespace overrides")
		require.Equal(t, "local-ns", handlerSaw)
	})

	t.Run("without translation configured the stamp is the whole chain", func(t *testing.T) {
		for _, callerNamesLocalNamespaces := range []bool{true, false} {
			require.Len(t, stampAndTranslate(callerNamesLocalNamespaces, nil), 1)
		}
	})
}

func TestBuildProxyServerEncryptsOnlyNonAdminTraffic(t *testing.T) {
	var intercepted []string
	encryptor := &interceptor.Encryptor{
		UnaryClientInterceptor: func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			intercepted = append(intercepted, method)
			return invoker(ctx, method, req, reply, cc, opts...)
		},
	}

	nsTranslations, err := (&config.StringTranslator{}).AsLocalToRemoteBiMap()
	require.NoError(t, err)

	upstream := &recordingConn{}
	server, err := buildProxyServer(serverConfiguration{
		name:           "test",
		directionLabel: "outbound",
		client:         upstream,
		managedClient:  &recordingConn{},
		encryptor:      encryptor,
		nsTranslations: nsTranslations,
		loggers:        logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{})),
	}, encryption.TLSConfig{}, func(int32, int32) {}, t.Context())
	require.NoError(t, err)

	listener, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	conn, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	_, err = workflowservice.NewWorkflowServiceClient(conn).
		StartWorkflowExecution(t.Context(), &workflowservice.StartWorkflowExecutionRequest{})
	require.NoError(t, err)
	_, err = operatorservice.NewOperatorServiceClient(conn).
		ListNexusEndpoints(t.Context(), &operatorservice.ListNexusEndpointsRequest{})
	require.NoError(t, err)
	_, err = adminservice.NewAdminServiceClient(conn).
		DeleteWorkflowExecution(t.Context(), &adminservice.DeleteWorkflowExecutionRequest{})
	require.NoError(t, err)

	require.Equal(t, []string{
		workflowservice.WorkflowService_StartWorkflowExecution_FullMethodName,
		operatorservice.OperatorService_ListNexusEndpoints_FullMethodName,
	}, intercepted, "admin traffic carries replication, which is not sealed here")
	require.Equal(t, []string{
		workflowservice.WorkflowService_StartWorkflowExecution_FullMethodName,
		operatorservice.OperatorService_ListNexusEndpoints_FullMethodName,
		adminservice.AdminService_DeleteWorkflowExecution_FullMethodName,
	}, upstream.invoked, "every call still has to reach the upstream")
}

// recordingWorkflowService remembers the last StartWorkflowExecution request it
// was sent, so a test can see what a payload looked like when it arrived.
type recordingWorkflowService struct {
	workflowservice.UnimplementedWorkflowServiceServer

	mu          sync.Mutex
	last        *workflowservice.StartWorkflowExecutionRequest
	queryResult *commonpb.Payload
}

func (s *recordingWorkflowService) StartWorkflowExecution(_ context.Context, req *workflowservice.StartWorkflowExecutionRequest) (*workflowservice.StartWorkflowExecutionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = req
	return &workflowservice.StartWorkflowExecutionResponse{}, nil
}

// QueryWorkflow answers with whatever setQueryResult left, or a plaintext result
// when it left nothing, so a test can see what a response payload looked like
// by the time it left the proxy.
func (s *recordingWorkflowService) QueryWorkflow(context.Context, *workflowservice.QueryWorkflowRequest) (*workflowservice.QueryWorkflowResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	result := s.queryResult
	if result == nil {
		result = plainPayload(`"answer"`)
	}
	return &workflowservice.QueryWorkflowResponse{QueryResult: &commonpb.Payloads{Payloads: []*commonpb.Payload{result}}}, nil
}

func (s *recordingWorkflowService) setQueryResult(p *commonpb.Payload) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queryResult = p
}

func (s *recordingWorkflowService) lastInput(t *testing.T) *commonpb.Payload {
	s.mu.Lock()
	defer s.mu.Unlock()
	require.NotNil(t, s.last, "no StartWorkflowExecution request arrived")
	require.Len(t, s.last.GetInput().GetPayloads(), 1)
	return s.last.GetInput().GetPayloads()[0]
}

func startRecordingWorkflowService(t *testing.T, address string) *recordingWorkflowService {
	listener, err := net.Listen("tcp", address)
	require.NoError(t, err)

	svc := &recordingWorkflowService{}
	server := grpc.NewServer()
	workflowservice.RegisterWorkflowServiceServer(server, svc)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	return svc
}

func plainPayload(data string) *commonpb.Payload {
	return &commonpb.Payload{
		Metadata: map[string][]byte{codec.MetadataEncoding: []byte("json/plain")},
		Data:     []byte(data),
	}
}

func workflowClient(t *testing.T, address string) workflowservice.WorkflowServiceClient {
	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return workflowservice.NewWorkflowServiceClient(conn)
}

func startWorkflowWithInput(t *testing.T, address string, input *commonpb.Payload) {
	_, err := workflowClient(t, address).StartWorkflowExecution(t.Context(),
		&workflowservice.StartWorkflowExecutionRequest{
			Namespace: "ns",
			Input:     &commonpb.Payloads{Payloads: []*commonpb.Payload{input}},
		})
	require.NoError(t, err)
}

// encryptingConnection is a cluster connection between two recording workflow
// services, with encryption configured under testEncryptionKeyURI.
type encryptingConnection struct {
	local, remote     *recordingWorkflowService
	outbound, inbound string
}

var testEncryptionKeyURI = "testing://" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))

func startEncryptingConnection(t *testing.T, enabled bool, edits ...func(*config.EncryptionConfig)) encryptingConnection {
	ports := getDynamicPorts(t, 4)
	localTemporalAddr, remoteTemporalAddr, proxyOutbound, proxyInbound := ports[0], ports[1], ports[2], ports[3]

	c := encryptingConnection{
		remote:   startRecordingWorkflowService(t, remoteTemporalAddr),
		local:    startRecordingWorkflowService(t, localTemporalAddr),
		outbound: proxyOutbound,
		inbound:  proxyInbound,
	}

	cfg := makeTCPClusterConfig("encrypting", localFVI, remoteFVI, proxyOutbound,
		localTemporalAddr, proxyInbound, proxyOutbound, remoteTemporalAddr)
	cfg.EncryptionConfig = config.EncryptionConfig{
		Enabled: enabled,
		Default: &config.KeyPolicy{URI: testEncryptionKeyURI, Duration: time.Hour},
	}
	for _, edit := range edits {
		edit(&cfg.EncryptionConfig)
	}

	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	cc, err := NewClusterConnection(t.Context(), cfg, nil, loggers)
	require.NoError(t, err)
	cc.Start()

	return c
}

// sealedPayload returns data the way the peer holds it: sealed on its way out
// through c.
func (c encryptingConnection) sealedPayload(t *testing.T, data string) *commonpb.Payload {
	startWorkflowWithInput(t, c.outbound, plainPayload(data))
	return c.remote.lastInput(t)
}

func queryResult(t *testing.T, address string) *commonpb.Payload {
	resp, err := workflowClient(t, address).QueryWorkflow(t.Context(), &workflowservice.QueryWorkflowRequest{Namespace: "ns"})
	require.NoError(t, err)
	require.Len(t, resp.GetQueryResult().GetPayloads(), 1)
	return resp.GetQueryResult().GetPayloads()[0]
}

func encodingOf(p *commonpb.Payload) string {
	return string(p.GetMetadata()[codec.MetadataEncoding])
}

func TestClusterConnectionEncryption(t *testing.T) {
	c := startEncryptingConnection(t, true)
	remote, local, proxyOutbound, proxyInbound := c.remote, c.local, c.outbound, c.inbound

	t.Run("outbound payloads reach the remote sealed", func(t *testing.T) {
		startWorkflowWithInput(t, proxyOutbound, plainPayload(`"secret"`))

		got := remote.lastInput(t)
		require.Equal(t, codec.EncryptionEncoding, string(got.GetMetadata()[codec.MetadataEncoding]))
		require.NotContains(t, string(got.GetData()), "secret")
	})

	t.Run("inbound payloads the peer holds sealed reach the local cluster opened", func(t *testing.T) {
		startWorkflowWithInput(t, proxyInbound, c.sealedPayload(t, `"round-trip"`))

		got := local.lastInput(t)
		require.Equal(t, "json/plain", string(got.GetMetadata()[codec.MetadataEncoding]))
		require.Equal(t, `"round-trip"`, string(got.GetData()))
	})

	t.Run("local responses reach the peer sealed", func(t *testing.T) {
		got := queryResult(t, proxyInbound)
		require.Equal(t, codec.EncryptionEncoding, encodingOf(got))
		require.NotContains(t, string(got.GetData()), "answer")
	})
}

func TestClusterConnectionEncryptionAlreadySealed(t *testing.T) {
	// The customer's codec encrypted these before they reached the proxy, so
	// they cross as they are, for the customer's codec to open again.
	c := startEncryptingConnection(t, true, func(ec *config.EncryptionConfig) {
		ec.AlreadySealedEncodings = []string{"binary/customer-encrypted"}
	})

	theirs := func() *commonpb.Payload {
		return &commonpb.Payload{
			Metadata: map[string][]byte{codec.MetadataEncoding: []byte("binary/customer-encrypted")},
			Data:     []byte("their-ciphertext"),
		}
	}

	t.Run("outbound", func(t *testing.T) {
		startWorkflowWithInput(t, c.outbound, theirs())
		require.True(t, proto.Equal(theirs(), c.remote.lastInput(t)))
	})

	t.Run("inbound responses", func(t *testing.T) {
		c.local.setQueryResult(theirs())
		require.True(t, proto.Equal(theirs(), queryResult(t, c.inbound)))
	})
}

func TestClusterConnectionEncryptionDisabled(t *testing.T) {
	// Disabled with keys still configured is what switching encryption off
	// leaves behind. Nothing is sealed any more, but what was sealed while it
	// was on still opens, in both directions. The payloads are sealed by an
	// enabled connection under the same key, as they would have been before.
	sealer := startEncryptingConnection(t, true)
	c := startEncryptingConnection(t, false)

	t.Run("outbound payloads reach the remote as sent", func(t *testing.T) {
		startWorkflowWithInput(t, c.outbound, plainPayload(`"plain"`))

		got := c.remote.lastInput(t)
		require.Equal(t, "json/plain", encodingOf(got))
		require.Equal(t, `"plain"`, string(got.GetData()))
	})

	t.Run("inbound payloads the peer holds sealed reach the local cluster opened", func(t *testing.T) {
		startWorkflowWithInput(t, c.inbound, sealer.sealedPayload(t, `"from-peer"`))

		got := c.local.lastInput(t)
		require.Equal(t, "json/plain", encodingOf(got))
		require.Equal(t, `"from-peer"`, string(got.GetData()))
	})

	t.Run("sealed responses from the remote are opened", func(t *testing.T) {
		c.remote.setQueryResult(sealer.sealedPayload(t, `"stored-sealed"`))

		got := queryResult(t, c.outbound)
		require.Equal(t, "json/plain", encodingOf(got))
		require.Equal(t, `"stored-sealed"`, string(got.GetData()))
	})

	t.Run("local responses reach the peer as sent", func(t *testing.T) {
		got := queryResult(t, c.inbound)
		require.Equal(t, "json/plain", encodingOf(got))
		require.Equal(t, `"answer"`, string(got.GetData()))
	})
}

func TestNewEncryptorsAreNilWithoutKeys(t *testing.T) {
	// No encryption block at all: nothing to seal or open with, and no reason to
	// visit a single payload.
	outbound, inbound, err := newEncryptors(t.Context(), config.EncryptionConfig{}, nil, log.NewTestLogger())
	require.NoError(t, err)
	require.Nil(t, outbound)
	require.Nil(t, inbound)
}
