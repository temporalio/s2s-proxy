package transport

import (
	"context"
	"fmt"
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

func testConnection(t *testing.T, clientTs TransportManager, serverTs TransportManager) {
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
		require.NoError(t, err)
		fmt.Println("--- grpc server stopped")
	}()

	conn, err := client.Connect()
	require.NoError(t, err)
	adminClient := adminservice.NewAdminServiceClient(conn)

	req := &adminservice.DescribeClusterRequest{}
	res, err := adminClient.DescribeCluster(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, clusterName, res.ClusterName)

	// This will close session.
	grpcServer.GracefulStop()
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

	servChan := make(chan error, 1)
	go func(done chan<- error) {
		done <- muxServer.Start()
	}(servChan)

	err := muxClient.Start()
	require.NoError(t, err)

	err = <-servChan
	require.NoError(t, err)

	defer func() {
		muxClient.Stop()
		muxServer.Stop()
	}()

	testConnection(t, muxClient, muxServer)

	// var cleanups []func()

	// cleanups = append(cleanups, testConnection(t, muxClient, muxServer))
	// testConnection(t, muxServer, muxClient)

	// for _, cleanup := range cleanups {
	// 	cleanup()
	// }
}

func TestConnect(t *testing.T) {
	logger := log.NewTestLogger()

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

	muxClient := NewTransportManager(config.NewMockConfigProvider(
		config.S2SProxyConfig{
			MuxTransports: []config.MuxTransportConfig{muxClientCfg},
		}), logger)

	muxServer := NewTransportManager(config.NewMockConfigProvider(
		config.S2SProxyConfig{
			MuxTransports: []config.MuxTransportConfig{muxServerCfg},
		}), logger)

	srvChan := make(chan error)
	shutdownChan := make(chan struct{})
	go func() {
		err := muxServer.Start()
		if err != nil {
			srvChan <- err
			fmt.Println("muxServer failed")
			return
		}

		fmt.Println("muxServer started: server", *muxServer.(*transportManagerImpl).muxTransports[muxedName])
		srvChan <- nil

		fmt.Println("muxServer wait for done")
		<-shutdownChan
		muxServer.Stop()
	}()

	err := muxClient.Start()
	require.NoError(t, err)
	err = <-srvChan
	require.NoError(t, err)

	fmt.Println("Test started: client", *muxClient.(*transportManagerImpl).muxTransports[muxedName])
	// testConnection(t, muxClient, muxServer)
	fmt.Println("Test done")
	shutdownChan <- struct{}{}
	muxClient.Stop()
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

	for i := 0; i < 5; i++ {
		fmt.Println(" --------------- ")
		testTransportWithCfg(t, muxClientCfg, muxServerCfg)
	}
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
