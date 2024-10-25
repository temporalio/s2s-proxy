package transport

import (
	"context"
	"os"
	"path/filepath"
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

func testMuxConnection(t *testing.T, clientTs TransportManager, serverTs TransportManager) {
	client, err := clientTs.CreateClientTransport(config.ClientConfig{
		Type:             config.MuxTransport,
		MuxTransportName: muxedName,
	})
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(grpcServer, &service{})
	server, err := serverTs.CreateServerTransport(config.ServerConfig{
		Type:             config.MuxTransport,
		MuxTransportName: muxedName,
	})
	require.NoError(t, err)

	go func() {
		err = server.Serve(grpcServer)
		if err != nil {
			// it is possible to receive an error here becaus we created two grpcServer:
			// one for normal direction; one for reverse direction and they all
			// share the same underly connection. If grpcServer A calls stop before
			// grpcServer B calls stop, grpcServer B can get an error here.
			t.Log("grpcServer received err", err)
		}
	}()

	conn, err := client.Connect()
	require.NoError(t, err)
	adminClient := adminservice.NewAdminServiceClient(conn)

	req := &adminservice.DescribeClusterRequest{}
	res, err := adminClient.DescribeCluster(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, clusterName, res.ClusterName)

	conn.Close()
	// Defer stopping grpcServer after test is done as GracefulStop will close underlying listener
	t.Cleanup(func() { grpcServer.GracefulStop() })
}

func testTransportWithCfg(t *testing.T, muxClientCfg config.MuxTransportConfig, muxServerCfg config.MuxTransportConfig) {
	logger := log.NewTestLogger()

	muxClient := NewTransportManager(config.NewMockConfigProvider(
		config.S2SProxyConfig{
			MuxTransports: []config.MuxTransportConfig{muxClientCfg},
		}), logger)

	muxServer := NewTransportManager(config.NewMockConfigProvider(
		config.S2SProxyConfig{
			MuxTransports: []config.MuxTransportConfig{muxServerCfg},
		}), logger)

	serverCh := make(chan error, 1)
	go func() {
		serverCh <- muxServer.Start()
	}()

	err := muxClient.Start()
	require.NoError(t, err)

	err = <-serverCh
	require.NoError(t, err)

	t.Cleanup(func() {
		muxClient.Stop()
		muxServer.Stop()
	})

	testMuxConnection(t, muxClient, muxServer)
	testMuxConnection(t, muxServer, muxClient)
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
