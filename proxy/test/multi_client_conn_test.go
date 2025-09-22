package proxy

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/proto/1_22/server/api/adminservice/v1"
	proxy "github.com/temporalio/s2s-proxy/testserver"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

func TestMultiClientConn(t *testing.T) {
	scenario := proxy.NewTestScenario(t, 10, log.NewTestLogger())
	connMap := make(map[string]func() (net.Conn, error), 5)
	mcc, err := grpcutil.NewMultiClientConn("testconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	require.NoError(t, err)
	for i := range 5 {
		key := fmt.Sprintf("mux-%d", i)
		connMap[key] = scenario.Muxes[i].ClientMux.Open
	}
	go func() {
		// This call will hang until UpdateState!
		requireCallWithNewClient(t, mcc)
		t.Log("Delayed call eventually succeeded!")
	}()
	t.Log("Delayed call won't fire yet")
	mcc.UpdateState(connMap)
	for range scenario.Muxes {
		requireCallWithNewClient(t, mcc)
	}
	for i := range 10 {
		key := fmt.Sprintf("mux-%d", i)
		connMap[key] = scenario.Muxes[i].ClientMux.Open
	}
	mcc.UpdateState(connMap)
	for range scenario.Muxes {
		requireCallWithNewClient(t, mcc)
	}
	mcc.UpdateState(make(map[string]func() (net.Conn, error)))
	timeout, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	client := adminservice.NewAdminServiceClient(mcc)
	_, err = client.DescribeCluster(timeout, &adminservice.DescribeClusterRequest{})
	require.Equal(t, status.Code(err), codes.Unavailable)
	cancel()
}

func requireCallWithNewClient(t *testing.T, mcc *grpcutil.MultiClientConn) {
	client := adminservice.NewAdminServiceClient(mcc)
	resp, err := client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
	require.NoError(t, err)
	t.Log("Connected to cluster", resp.ClusterName)
}
