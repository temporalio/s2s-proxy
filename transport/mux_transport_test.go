package transport

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
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
		Client: &config.TCPClientSetting{
			ServerAddress: serverAddress,
		},
	}

	testMuxServerCfg = config.MuxTransportConfig{
		Name: muxedName,
		Mode: config.ServerMode,
		Server: &config.TCPServerSetting{
			ListenAddress: serverAddress,
		},
	}

	testLogger = log.NewTestLogger()
)

type service struct {
	adminservice.UnimplementedAdminServiceServer
}

func (s *service) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	return &adminservice.DescribeClusterResponse{
		ClusterName: clusterName,
	}, nil
}

func connect(t *testing.T, clientCM *MuxConnectMananger, serverCM *MuxConnectMananger) (MuxTransport, MuxTransport) {
	var clientTs MuxTransport
	var serverTs MuxTransport

	// Establish connection
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		clientTs, err = clientCM.Open()
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		var err error
		serverTs, err = serverCM.Open()
		require.NoError(t, err)
	}()

	wg.Wait()
	return clientTs, serverTs
}

func startServer(t *testing.T, serverTs ServerTransport) *grpc.Server {
	grpcServer := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(grpcServer, &service{})
	go func() {
		err := serverTs.Serve(grpcServer)
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

func testClient(t *testing.T, clientTs ClientTransport) {
	conn, err := clientTs.Connect()
	require.NoError(t, err)
	adminClient := adminservice.NewAdminServiceClient(conn)

	req := &adminservice.DescribeClusterRequest{}
	res, err := adminClient.DescribeCluster(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, clusterName, res.ClusterName)

	conn.Close()
}

func testMuxConnection(t *testing.T, muxClientCfg config.MuxTransportConfig, muxServerCfg config.MuxTransportConfig, repeat int) {
	clientCM := newMuxConnectManager(muxClientCfg, testLogger)
	serverCM := newMuxConnectManager(muxServerCfg, testLogger)

	require.NoError(t, clientCM.start())
	require.NoError(t, serverCM.start())

	defer func() {
		clientCM.stop()
		serverCM.stop()
	}()

	for i := 0; i < repeat; i++ {
		t.Log("Test connection", "repeat", i)
		clientTs, serverTs := connect(t, clientCM, serverCM)

		server := startServer(t, serverTs)
		testClient(t, clientTs)

		server.GracefulStop()

		clientTs.Close()
		serverTs.Close()
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
	os.Chdir(filepath.Join("..", "develop"))
	defer func() {
		os.Chdir(pwd)
	}()

	muxClientCfgTLS := config.MuxTransportConfig{
		Name: muxedName,
		Mode: config.ClientMode,
		Client: &config.TCPClientSetting{
			ServerAddress: serverAddress,
			TLS: encryption.ClientTLSConfig{
				CertificatePath: filepath.Join("certificates", "proxy2.pem"),
				KeyPath:         filepath.Join("certificates", "proxy2.key"),
				ServerName:      "onebox-proxy1.cluster.tmprl.cloud",
				ServerCAPath:    filepath.Join("certificates", "proxy1.pem"),
			},
		},
	}

	muxServerCfgTLS := config.MuxTransportConfig{
		Name: muxedName,
		Mode: config.ServerMode,
		Server: &config.TCPServerSetting{
			ListenAddress: serverAddress,
			TLS: encryption.ServerTLSConfig{
				CertificatePath:   filepath.Join("certificates", "proxy1.pem"),
				KeyPath:           filepath.Join("certificates", "proxy1.key"),
				ClientCAPath:      filepath.Join("certificates", "proxy2.pem"),
				RequireClientAuth: true,
			},
		},
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

func TestMuxTransporWaitForClose(t *testing.T) {
	testClose := func(t *testing.T, closeCfg config.MuxTransportConfig, waitForCloseCfg config.MuxTransportConfig) {
		closeCM := newMuxConnectManager(closeCfg, testLogger)
		waitForCloseCM := newMuxConnectManager(waitForCloseCfg, testLogger)
		require.NoError(t, closeCM.start())
		require.NoError(t, waitForCloseCM.start())

		defer func() {
			closeCM.stop()
			waitForCloseCM.stop()
		}()

		closeTs, waitForCloseTs := connect(t, closeCM, waitForCloseCM)
		closeTs.Close()
		_, ok := <-closeTs.CloseChan()
		require.False(t, ok)

		// waitForClose transport should be closed.
		_, ok = <-waitForCloseTs.CloseChan()
		require.False(t, ok)

		// Reconnect transport
		closeTs, waitForCloseTs = connect(t, closeCM, waitForCloseCM)
	}

	runTests(t, testClose)
}

func TestMuxTransporReconnect(t *testing.T) {
	testReconnect := func(t *testing.T, clientCfg config.MuxTransportConfig, serverCfg config.MuxTransportConfig) {
		clientCM := newMuxConnectManager(clientCfg, testLogger)
		serverCM := newMuxConnectManager(serverCfg, testLogger)

		require.NoError(t, clientCM.start())
		require.NoError(t, serverCM.start())

		// Connect first
		clientTs, serverTs := connect(t, clientCM, serverCM)

		// close underlying connection
		clientCM.stop()

		// Wait for both transport to close
		<-clientTs.CloseChan()
		<-serverTs.CloseChan()

		// restart clientCM and connect again
		clientCM.start()
		clientTs, serverTs = connect(t, clientCM, serverCM)

		clientCM.stop()
		serverCM.stop()
	}

	runTests(t, testReconnect)
}

func TestMuxTransportMultiServer(t *testing.T) {
	testMulti := func(t *testing.T, clientCfg config.MuxTransportConfig, serverCfg config.MuxTransportConfig) {
		clientCM := newMuxConnectManager(clientCfg, testLogger)
		serverCM := newMuxConnectManager(serverCfg, testLogger)

		require.NoError(t, clientCM.start())
		require.NoError(t, serverCM.start())

		clientTs, serverTs := connect(t, clientCM, serverCM)

		// Start server on both sides
		server1 := startServer(t, clientTs)
		server2 := startServer(t, serverTs)

		testClient(t, clientTs)
		testClient(t, serverTs)

		server1.GracefulStop()
		server2.GracefulStop()

		clientCM.stop()
		serverCM.stop()
	}

	runTests(t, testMulti)
}
