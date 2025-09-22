package proxy

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/temporalio/s2s-proxy/proto/1_22/server/api/adminservice/v1"
	proxy "github.com/temporalio/s2s-proxy/testserver"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

func TestMultiClientConn(t *testing.T) {
	scenario := proxy.NewTestScenario(t, 10, log.NewTestLogger())
	connMap := make(map[string]func() (net.Conn, error), 5)
	connMapHolder := &connMap
	mcc, err := grpcutil.NewMultiClientConn("testconn",
		func() (map[string]func() (net.Conn, error), error) { return *connMapHolder, nil },
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	require.NoError(t, err)
	for i := range 5 {
		key := fmt.Sprintf("mux-%d", i)
		connMap[key] = scenario.Muxes[i].ClientMux.Open
	}
	// Not functioning yet...
	//mcc.NotifyNewConnections()
	for range scenario.Muxes {
		client := adminservice.NewAdminServiceClient(mcc)
		resp, err := client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		require.NoError(t, err)
		t.Log("Connected to cluster", resp.ClusterName)
	}
	require.NoError(t, err)
}
