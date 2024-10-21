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

func testMultiplex(t *testing.T, clientProvider TransportProvider, serverProvider TransportProvider) {
	// client
	client, err := clientProvider.CreateClientTransport(config.ClientConfig{
		Type:            config.MultiplexTransport,
		MultiplexerName: muxedName,
	})
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(grpcServer, &service{})
	server, err := serverProvider.CreateServerTransport(config.ServerConfig{
		Type:            config.MultiplexTransport,
		MultiplexerName: muxedName,
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
}

func TestMultiplexTransport(t *testing.T) {
	var providerA, providerB TransportProvider

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		var err error
		providerA, err = NewTransprotProvider(config.NewMockConfigProvider(
			config.S2SProxyConfig{
				MultiplexTransports: []config.MultiplexTransportConfig{
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
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()

		var err error
		providerB, err = NewTransprotProvider(config.NewMockConfigProvider(
			config.S2SProxyConfig{
				MultiplexTransports: []config.MultiplexTransportConfig{
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
		require.NoError(t, err)
	}()

	wg.Wait()
	testMultiplex(t, providerA, providerB)
	testMultiplex(t, providerB, providerA)
}
