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

type service struct {
	adminservice.UnimplementedAdminServiceServer
}

func (s *service) DescribeCluster(ctx context.Context, in0 *adminservice.DescribeClusterRequest) (*adminservice.DescribeClusterResponse, error) {
	return &adminservice.DescribeClusterResponse{
		ClusterName: clusterName,
	}, nil
}

func testMuxConnection(t *testing.T, clientTransManager TransportManager, serverTransManager TransportManager) {
	var clientTs MuxTransport
	var serverTs MuxTransport

	// Establish connection
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		var err error
		clientTs, err = clientTransManager.Open(muxedName)
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		var err error
		serverTs, err = serverTransManager.Open(muxedName)
		require.NoError(t, err)
	}()

	wg.Wait()

	defer func() {
		serverTs.Close()
		clientTs.Close()
	}()

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

	defer grpcServer.GracefulStop()

	conn, err := clientTs.Connect()
	require.NoError(t, err)
	adminClient := adminservice.NewAdminServiceClient(conn)

	req := &adminservice.DescribeClusterRequest{}
	res, err := adminClient.DescribeCluster(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, clusterName, res.ClusterName)

	conn.Close()
}

func testTransportWithCfg(t *testing.T, muxClientCfg config.MuxTransportConfig, muxServerCfg config.MuxTransportConfig) {
	logger := log.NewTestLogger()

	muxClientManager := NewTransportManager(config.NewMockConfigProvider(
		config.S2SProxyConfig{
			MuxTransports: []config.MuxTransportConfig{muxClientCfg},
		}), logger)

	muxServerManager := NewTransportManager(config.NewMockConfigProvider(
		config.S2SProxyConfig{
			MuxTransports: []config.MuxTransportConfig{muxServerCfg},
		}), logger)

	testMuxConnection(t, muxClientManager, muxServerManager)
}

func TestMuxTransport(t *testing.T) {
	muxClientCfg := config.MuxTransportConfig{
		Name: muxedName,
		Mode: config.ClientMode,
		Client: &config.TCPClientSetting{
			ServerAddress: serverAddress,
		},
	}

	muxServerCfg := config.MuxTransportConfig{
		Name: muxedName,
		Mode: config.ServerMode,
		Server: &config.TCPServerSetting{
			ListenAddress: serverAddress,
		},
	}

	testTransportWithCfg(t, muxClientCfg, muxServerCfg)
}

func TestMuxTransportTLS(t *testing.T) {
	pwd, _ := os.Getwd()
	os.Chdir(filepath.Join("..", "develop"))
	defer func() {
		os.Chdir(pwd)
	}()

	muxClientCfg := config.MuxTransportConfig{
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

	muxServerCfg := config.MuxTransportConfig{
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

	testTransportWithCfg(t, muxClientCfg, muxServerCfg)
}
