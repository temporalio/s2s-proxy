package transport

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/api/adminservice/v1"
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

func testConnection(t *testing.T, clientTs TransportManager, serverTs TransportManager) func() {
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
	}()

	conn, err := client.Connect()
	require.NoError(t, err)
	adminClient := adminservice.NewAdminServiceClient(conn)

	req := &adminservice.DescribeClusterRequest{}
	res, err := adminClient.DescribeCluster(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, clusterName, res.ClusterName)

	// defer stop grpcServer after test is complete because GracefulStop will
	// close listener, which will close mux session.
	return func() {
		grpcServer.GracefulStop()
	}
}

func TestMuxTransport(t *testing.T) {
	tsA := NewTransportManager(config.NewMockConfigProvider(
		config.S2SProxyConfig{
			MuxTransports: []config.MuxTransportConfig{
				{
					Name: muxedName,
					Mode: config.ClientMode,
					Client: &config.TCPClientSetting{
						ServerAddress: serverAddress,
					},
				},
			},
		},
	))

	tsB := NewTransportManager(config.NewMockConfigProvider(
		config.S2SProxyConfig{
			MuxTransports: []config.MuxTransportConfig{
				{
					Name: muxedName,
					Mode: config.ServerMode,
					Server: &config.TCPServerSetting{
						ListenAddress: serverAddress,
					},
				},
			},
		},
	))

	// Establish mux session between tsA and tsB concurrently
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		err := tsA.Start()
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		err := tsB.Start()
		require.NoError(t, err)
	}()
	wg.Wait()

	defer func() {
		tsA.Stop()
		tsB.Stop()
	}()

	var closers []func()
	closers = append(closers, testConnection(t, tsA, tsB))
	closers = append(closers, testConnection(t, tsB, tsA))

	for _, close := range closers {
		close()
	}
}
