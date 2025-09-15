package transport

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

const (
	serverAddress = "localhost:5333"
	clusterName   = "test_cluster"
	muxedName     = "muxed"
)

var (
	testMuxClientCfg = config.MuxTransportConfig{
		Name: muxedName,
		Mode: config.ClientMode,
		Client: config.TCPClientSetting{
			ServerAddress: serverAddress,
		},
	}

	testMuxServerCfg = config.MuxTransportConfig{
		Name: muxedName,
		Mode: config.ServerMode,
		Server: config.TCPServerSetting{
			ListenAddress: serverAddress,
		},
	}

	testLogger log.Logger
)

func init() {
	// Uncomment for debug logs
	//_ = os.Setenv("TEMPORAL_TEST_LOG_LEVEL", "debug")
	_ = os.Setenv("TEMPORAL_TEST_LOG_LEVEL", "error")
	testLogger = log.NewTestLogger()
}

type service struct {
	adminservice.UnimplementedAdminServiceServer
}

func (s *service) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	return &adminservice.DescribeClusterResponse{
		ClusterName: clusterName,
	}, nil
}

func connect(t *testing.T, clientConfig config.MuxTransportConfig, serverConfig config.MuxTransportConfig) (mux.MuxManager, mux.MuxManager) {
	clientMgr, err := mux.NewMuxManager(clientConfig, testLogger)
	mux.SetCustomWakeInterval(clientMgr, 5*time.Millisecond)
	require.NoError(t, err)
	clientMgr.Start()
	serverMgr, err := mux.NewMuxManager(serverConfig, testLogger)
	mux.SetCustomWakeInterval(serverMgr, 5*time.Millisecond)
	require.NoError(t, err)
	serverMgr.Start()
	return clientMgr, serverMgr
}

func startServer(t *testing.T, serverMuxMgr ServerTransport) *grpc.Server {
	grpcServer := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(grpcServer, &service{})
	go func() {
		err := serverMuxMgr.Serve(grpcServer)
		if err != nil {
			// it is possible to receive an error here because test creates two grpcServers:
			// one on muxServer and one on muxClient based on the same connection between
			// muxClient to muxServer. If one of the grpcServer stopped before the other one (which
			// close the underly TCP connection), the other grpcServer can get an error here.
			t.Log("grpcServer received err", err)
		}
	}()

	return grpcServer
}

func testClient(t *testing.T, clientMuxMgr ClientTransport) {
	conn, err := clientMuxMgr.Connect(metrics.GRPCInboundClientMetrics)
	require.NoError(t, err)
	adminClient := adminservice.NewAdminServiceClient(conn)

	req := &adminservice.DescribeClusterRequest{}
	res, err := adminClient.DescribeCluster(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, clusterName, res.ClusterName)

	require.NoError(t, conn.Close())
}

func testMuxConnection(t *testing.T, muxClientCfg config.MuxTransportConfig, muxServerCfg config.MuxTransportConfig, repeat int) {
	for i := 0; i < repeat; i++ {
		clientMuxMgr, serverMuxMgr := connect(t, muxClientCfg, muxServerCfg)

		server := startServer(t, serverMuxMgr)
		testClient(t, clientMuxMgr)

		server.GracefulStop()

		clientMuxMgr.Close()
		require.True(t, clientMuxMgr.IsClosed())
		serverMuxMgr.Close()
		require.True(t, serverMuxMgr.IsClosed())
	}
}

func testMuxConnectionWithConfig(t *testing.T, muxClientCfg config.MuxTransportConfig, muxServerCfg config.MuxTransportConfig) {
	repeat := 10

	t.Run("client call server", func(t *testing.T) {
		testMuxConnection(t, muxClientCfg, muxServerCfg, repeat)
	})

	muxClientCfg.Name += "-reverse"
	muxServerCfg.Name += "-reverse"
	t.Run("server call client", func(t *testing.T) {
		testMuxConnection(t, muxServerCfg, muxClientCfg, repeat)
	})
}

func TestMuxTransport(t *testing.T) {
	testMuxConnectionWithConfig(t, testMuxClientCfg, testMuxServerCfg)
}

func TestMuxTransportTLS(t *testing.T) {
	pwd, _ := os.Getwd()
	require.NoError(t, os.Chdir(filepath.Join("..", "develop")))
	defer func() {
		require.NoError(t, os.Chdir(pwd))
	}()

	muxClientCfgTLS := testMuxClientCfg
	muxClientCfgTLS.Client.TLS = encryption.ClientTLSConfig{
		CertificatePath: filepath.Join("certificates", "proxy2.pem"),
		KeyPath:         filepath.Join("certificates", "proxy2.key"),
		ServerName:      "onebox-proxy1.cluster.tmprl.cloud",
		ServerCAPath:    filepath.Join("certificates", "proxy1.pem"),
	}
	muxServerCfgTLS := testMuxServerCfg
	muxServerCfgTLS.Server.TLS = encryption.ServerTLSConfig{
		CertificatePath:   filepath.Join("certificates", "proxy1.pem"),
		KeyPath:           filepath.Join("certificates", "proxy1.key"),
		ClientCAPath:      filepath.Join("certificates", "proxy2.pem"),
		RequireClientAuth: true,
	}

	testMuxConnectionWithConfig(t, muxClientCfgTLS, muxServerCfgTLS)
}

func runTests(t *testing.T, f func(t *testing.T, closeCfg config.MuxTransportConfig, waitForCloseCfg config.MuxTransportConfig)) {
	t.Run("client/server", func(t *testing.T) {
		f(t, testMuxClientCfg, testMuxServerCfg)
	})

	t.Run("server/client", func(t *testing.T) {
		f(t, testMuxServerCfg, testMuxClientCfg)
	})
}

func TestMuxTransportWaitForClose(t *testing.T) {
	testClose := func(t *testing.T, closeCfg config.MuxTransportConfig, waitForCloseCfg config.MuxTransportConfig) {
		closeTs, waitForCloseTs := connect(t, closeCfg, waitForCloseCfg)
		// Kill client-side mux
		closeTs.Close()
		_, ok := <-closeTs.CloseChan()
		require.False(t, ok)

		timeoutCtx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)

		require.Error(t, waitForCloseTs.PingSession(timeoutCtx), "waitForClose should be disconnected")
		// Other side should have disconnected, but muxManager won't be closed!
		connActive, _ := waitForCloseTs.WithConnection(timeoutCtx, func(swc *mux.SessionWithConn) (any, error) {
			return true, nil
		})
		// Should have gotten uninitialized value
		require.Nil(t, connActive)

		// clean up
		waitForCloseTs.Close()
		cancel()
	}

	runTests(t, testClose)
}

func TestMuxTransportStopConnectionManager(t *testing.T) {
	testReconnect := func(t *testing.T, clientCfg config.MuxTransportConfig, serverCfg config.MuxTransportConfig) {

		// Connect first
		clientMuxMgr, serverMuxMgr := connect(t, clientCfg, serverCfg)

		// close underlying connection
		clientMuxMgr.Close()
		serverMuxMgr.Close()

		// restart CM should fail
		clientMuxMgr.Start()
		require.True(t, clientMuxMgr.IsClosed())

		serverMuxMgr.Start()
		require.True(t, serverMuxMgr.IsClosed())
	}

	runTests(t, testReconnect)
}

func TestMuxTransportMultiServers(t *testing.T) {
	testMulti := func(t *testing.T, clientCfg config.MuxTransportConfig, serverCfg config.MuxTransportConfig) {
		clientMuxMgr, serverMuxMgr := connect(t, clientCfg, serverCfg)

		// Start server on both sides
		server1 := startServer(t, clientMuxMgr)
		server2 := startServer(t, serverMuxMgr)

		testClient(t, clientMuxMgr)
		testClient(t, serverMuxMgr)

		server1.GracefulStop()
		server2.GracefulStop()

		clientMuxMgr.Close()
		serverMuxMgr.Close()
	}

	runTests(t, testMulti)
}

func TestMuxTransportFailedToOpen(t *testing.T) {
	testMulti := func(t *testing.T, clientCfg config.MuxTransportConfig, serverCfg config.MuxTransportConfig) {
		clientCM, clientErr := mux.NewMuxManager(clientCfg, testLogger)
		serverCM, serverErr := mux.NewMuxManager(serverCfg, testLogger)

		require.NoError(t, clientErr)
		require.NoError(t, serverErr)

		clientCM.Close()
		serverCM.Close()

		clientCM.Start()
		require.True(t, clientCM.IsClosed())

		serverCM.Start()
		require.True(t, serverCM.IsClosed())
	}

	runTests(t, testMulti)
}
