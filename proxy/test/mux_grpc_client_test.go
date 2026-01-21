package proxy

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"

	"github.com/temporalio/s2s-proxy/endtoendtest"
)

var testLogger = log.NewTestLogger()

// Using separate ClientConn objects created using grpc.NewClientConn "just works", but the clients are completely
// separate and do not load-balance. We have to do the work of swapping out the client objects in the calling code.
func TestMultiClientMultiServer(t *testing.T) {
	scenario := endtoendtest.NewYamuxGRPCScenario(t, 10, testLogger)
	for i, muxClient := range scenario.Clients {
		resp, err := muxClient.Client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("adminService on mux %d", i), resp.ClusterName, "Invalid or missing cluster name")
	}
	scenario.Close()
}

// Using a single ClientConn object and naively swapping in a round-robin Dialer does not work, because
// the connection is cached.
func TestSingleClientCustomDialer(t *testing.T) {
	scenario := endtoendtest.NewYamuxGRPCScenario(t, 10, testLogger)
	counter := &atomic.Uint32{}
	counter.Store(0)
	superClientConn, err := grpc.NewClient("passthrough:unused",
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			//t.Log("Dialed addr", addr)
			return scenario.Muxes[counter.Add(1)%10].ClientMux.Open()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	for range 10 {
		resp, err := adminservice.NewAdminServiceClient(superClientConn).DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		require.NoError(t, err)
		require.Equal(t, "adminService on mux 1", resp.ClusterName)
	}
	scenario.Close()
}

// We require a custom dialer at all times, because our dialer needs to return the right mux connection.
// The dialer function only takes a string address and a context. The Balancer and Resolver don't have access
// to the context passed to Dial, which means our only real input is the address. Luckily, we have the local context
// the Dialer was constructed in, which means we can use a map from address -> mux-conn. The manual resolver will be
// notified with one endpoint per mux map key, and those map keys will arrive at the dialer.
func TestSingleClientCustomResolverAndDialer(t *testing.T) {
	scenario := endtoendtest.NewYamuxGRPCScenario(t, 10, testLogger)
	manualResolver := manual.NewBuilderWithScheme("multimux")
	// There is a note in resolver.State that mentions how Addresses and Endpoints work:
	//  //If a resolver sets Addresses but does not set Endpoints, one Endpoint
	//	// will be created for each Address before the State is passed to the LB
	//	// policy.  The BalancerAttributes of each entry in Addresses will be set
	//	// in Endpoints.Attributes, and be cleared in the Endpoint's Address's
	//	// BalancerAttributes.
	//	//
	//	// Soon, Addresses will be deprecated and replaced fully by Endpoints.
	//addresses := []resolver.Address{
	//	{Addr: "0"},
	//	{Addr: "1"},
	//	{Addr: "2"},
	//	{Addr: "3"},
	//	{Addr: "4"},
	//}
	manualResolver.InitialState(resolver.State{
		//Addresses: addresses,
		Endpoints: []resolver.Endpoint{
			{Addresses: []resolver.Address{{Addr: "0"}}},
			{Addresses: []resolver.Address{{Addr: "1"}}},
			{Addresses: []resolver.Address{{Addr: "2"}}},
			{Addresses: []resolver.Address{{Addr: "3"}}},
			{Addresses: []resolver.Address{{Addr: "4"}}},
		},
	})
	connSeen := make([]atomic.Bool, 10)
	superClientConn, err := grpc.NewClient("multimux://unused",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithResolvers(manualResolver),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			i, err := strconv.Atoi(addr)
			//t.Log("Dialed addr", addr)
			require.NoError(t, err)
			connSeen[i].Store(true)
			return scenario.Muxes[i].ClientMux.Open()
		}),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	require.NoError(t, err)
	t.Log("Checking clients 0-5")
	for range 5 {
		// Creating a new client is guaranteed to re-fetch the connection. Making multiple calls with one client
		// is not.
		client := adminservice.NewAdminServiceClient(superClientConn)
		_, err := client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		require.NoError(t, err)
		//t.Log("Got cluster", resp.ClusterName)
	}
	for i := range 5 {
		assert.Truef(t, connSeen[i].Load(), "Should have seen connection on %d", i)
	}
	manualResolver.UpdateState(resolver.State{
		Endpoints: []resolver.Endpoint{
			{Addresses: []resolver.Address{{Addr: "0"}}},
			{Addresses: []resolver.Address{{Addr: "1"}}},
			{Addresses: []resolver.Address{{Addr: "2"}}},
			{Addresses: []resolver.Address{{Addr: "3"}}},
			{Addresses: []resolver.Address{{Addr: "4"}}},
			{Addresses: []resolver.Address{{Addr: "5"}}},
			{Addresses: []resolver.Address{{Addr: "6"}}},
			{Addresses: []resolver.Address{{Addr: "7"}}},
			{Addresses: []resolver.Address{{Addr: "8"}}},
			{Addresses: []resolver.Address{{Addr: "9"}}},
		},
	})
	// The connections for muxes 0-4 are cached!! Don't reset connSeen
	//for i := range connSeen {
	//	connSeen[i].Store(false)
	//}

	t.Log("Checking clients 0-10")
	for range 10 {
		client := adminservice.NewAdminServiceClient(superClientConn)
		_, err := client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		require.NoError(t, err)
		//t.Log("Got cluster", resp.ClusterName)
	}
	for i := range 10 {
		assert.Truef(t, connSeen[i].Load(), "Should have seen connection on %d", i)
	}
	scenario.Close()
}

func TestSingleClientCustomResolverUpdate(t *testing.T) {
	scenario := endtoendtest.NewYamuxGRPCScenario(t, 10, testLogger)
	connSeen := make([]atomic.Bool, 10)
	manualResolver := manual.NewBuilderWithScheme("multimux")
	manualResolver.InitialState(resolver.State{
		Endpoints: []resolver.Endpoint{
			{Addresses: []resolver.Address{{Addr: "0"}}},
		},
	})
	superClientConn, err := grpc.NewClient("multimux://unused",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithResolvers(manualResolver),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			i, err := strconv.Atoi(addr)
			//t.Log("Dialed addr", addr)
			require.NoError(t, err)
			connSeen[i].Store(true)
			return scenario.Muxes[i].ClientMux.Open()
		}),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	require.NoError(t, err)
	stopClientThread := atomic.Bool{}
	grpcClient := adminservice.NewAdminServiceClient(superClientConn)
	go func() {
		for !stopClientThread.Load() {
			_, err := grpcClient.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
			require.NoError(t, err)
		}
	}()
	require.Eventually(t, connSeen[0].Load, 1*time.Second, 10*time.Millisecond)
	manualResolver.UpdateState(resolver.State{
		Endpoints: []resolver.Endpoint{
			{Addresses: []resolver.Address{{Addr: "0"}}},
			{Addresses: []resolver.Address{{Addr: "1"}}},
			{Addresses: []resolver.Address{{Addr: "2"}}},
			{Addresses: []resolver.Address{{Addr: "3"}}},
			{Addresses: []resolver.Address{{Addr: "4"}}},
			{Addresses: []resolver.Address{{Addr: "5"}}},
			{Addresses: []resolver.Address{{Addr: "6"}}},
			{Addresses: []resolver.Address{{Addr: "7"}}},
			{Addresses: []resolver.Address{{Addr: "8"}}},
			{Addresses: []resolver.Address{{Addr: "9"}}},
		},
	})
	allConnsSeen := func() bool {
		for i := range connSeen {
			if !connSeen[i].Load() {
				return false
			}
		}
		return true
	}
	require.Eventually(t, allConnsSeen, 1*time.Second, 10*time.Millisecond)
}
