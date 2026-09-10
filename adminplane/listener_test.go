package adminplane

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
)

const unparseableAddress = "127.0.0.1:\n6061"

type stubAdminService struct {
	proxyadminv1.UnimplementedProxyAdminServiceServer
	response *proxyadminv1.DescribeClusterConnectionsResponse
}

func (s *stubAdminService) DescribeClusterConnections(
	context.Context,
	*proxyadminv1.DescribeClusterConnectionsRequest,
) (*proxyadminv1.DescribeClusterConnectionsResponse, error) {
	return s.response, nil
}

func startStubServer(t *testing.T, response *proxyadminv1.DescribeClusterConnectionsResponse) (string, context.CancelFunc) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	grpcServer := grpc.NewServer()
	proxyadminv1.RegisterProxyAdminServiceServer(grpcServer, &stubAdminService{response: response})

	lifetime, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	NewServer("stub", lifetime, listener, grpcServer, log.NewTestLogger()).Start()

	return listener.Addr().String(), stop
}

func describeVia(t *testing.T, address string) (*proxyadminv1.DescribeClusterConnectionsResponse, error) {
	t.Helper()

	cc, release, err := DialOnce(address, nil)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	return proxyadminv1.NewProxyAdminServiceClient(cc).
		DescribeClusterConnections(ctx, &proxyadminv1.DescribeClusterConnectionsRequest{})
}

func TestDialOnceReachesAServer(t *testing.T) {
	address, _ := startStubServer(t, &proxyadminv1.DescribeClusterConnectionsResponse{
		ClusterConnections: []*proxyadminv1.ClusterConnection{
			{Name: "alpha", State: proxyadminv1.ConnectionState_CONNECTION_STATE_CONNECTED},
		},
	})

	response, err := describeVia(t, address)
	require.NoError(t, err)
	require.Len(t, response.ClusterConnections, 1)
	require.Equal(t, "alpha", response.ClusterConnections[0].Name)
	require.Equal(t, proxyadminv1.ConnectionState_CONNECTION_STATE_CONNECTED, response.ClusterConnections[0].State)
}

func TestDialOnceCapsTheResponseSizeAtFourMiB(t *testing.T) {
	const fourMiB = 4 * 1024 * 1024

	for _, tc := range []struct {
		name     string
		bytes    int
		wantCode codes.Code
	}{
		{name: "under four MiB", bytes: fourMiB - 1024, wantCode: codes.OK},
		{name: "over four MiB", bytes: fourMiB, wantCode: codes.ResourceExhausted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			address, _ := startStubServer(t, &proxyadminv1.DescribeClusterConnectionsResponse{
				ClusterConnections: []*proxyadminv1.ClusterConnection{
					{Name: strings.Repeat("a", tc.bytes)},
				},
			})

			_, err := describeVia(t, address)
			require.Equal(t, tc.wantCode, status.Code(err))
		})
	}
}

func TestDialOnceReturnsAReleaseFuncOnError(t *testing.T) {
	cc, release, err := DialOnce(unparseableAddress, nil)
	require.Error(t, err)
	require.Nil(t, cc)
	require.NotNil(t, release)
	require.NotPanics(t, release)
}

func TestServerStopsWhenItsLifetimeEnds(t *testing.T) {
	address, stop := startStubServer(t, &proxyadminv1.DescribeClusterConnectionsResponse{})

	response, err := describeVia(t, address)
	require.NoError(t, err)
	require.Empty(t, response.ClusterConnections)

	stop()

	require.Eventually(t, func() bool {
		listener, err := net.Listen("tcp", address)
		if err != nil {
			return false
		}
		return listener.Close() == nil
	}, 10*time.Second, 10*time.Millisecond)
}
