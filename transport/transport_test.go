package transport

import (
	"context"
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

func testMultiplex(t *testing.T, clientProvider *TransportProvider, serverProvider *TransportProvider) {
	client, err := serverProvider.CreateClientTransport(config.ClientConfig{
		Type:            config.MultiplexTransport,
		MultiplexerName: muxedName,
	},
	)
	require.NoError(t, err)

	server, err := clientProvider.CreateServerTransport(config.ServerConfig{
		Type:            config.MultiplexTransport,
		MultiplexerName: muxedName,
	})

	grpcServer := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(grpcServer, &service{})

	go func() {
		err := server.Serve(grpcServer)
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
	clientProvider := NewTranprotProvider(config.MultiplexTransportConfig{
		Clients: []config.MultiplexClientSetting{
			{
				Name: muxedName,
				TCPClientSetting: config.TCPClientSetting{
					ServerAddress: serverAddress,
				},
			},
		},
		Servers: []config.MultiplexServerSetting{},
	})

	serverProvider := NewTranprotProvider(config.MultiplexTransportConfig{
		Servers: []config.MultiplexServerSetting{
			{
				Name: muxedName,
				TCPServerSetting: config.TCPServerSetting{
					ListenAddress: serverAddress,
				},
			},
		},
	})

	go serverProvider.Init()
	err := clientProvider.Init()
	require.NoError(t, err)

	require.NoError(t, err)
	testMultiplex(t, clientProvider, serverProvider)
	testMultiplex(t, serverProvider, clientProvider)
}
