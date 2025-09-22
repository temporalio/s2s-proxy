package proxy

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"golang.org/x/sync/semaphore"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/proto/1_22/server/api/adminservice/v1"
	"github.com/temporalio/s2s-proxy/testserver"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

func TestMultiClientConn(t *testing.T) {
	scenario := testserver.NewTestScenario(t, 10, log.NewTestLogger())
	mcc, err := grpcutil.NewMultiClientConn("testconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	require.NoError(t, err)
	connMap := map[string]func() (net.Conn, error){"Mux": scenario.Muxes[0].ClientMux.Open}
	mcc.UpdateState(connMap)
	requireCallWithNewClient(t, mcc)
}

func TestMultiClientWaitsForResolve(t *testing.T) {
	scenario := testserver.NewTestScenario(t, 10, log.NewTestLogger())
	mcc, err := grpcutil.NewMultiClientConn("testconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	require.NoError(t, err)
	done := make(chan struct{})
	go func() {
		// This call will hang until UpdateState!
		requireCallWithNewClient(t, mcc)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("Call should be blocked on resolver initialization")
	case <-time.After(time.Millisecond * 100):
	}
	connMap := map[string]func() (net.Conn, error){"a mux!": scenario.Muxes[0].ClientMux.Open}
	mcc.UpdateState(connMap)
	select {
	case <-done:
	case <-time.After(time.Millisecond * 100):
		t.Fatal("Should have seen delayed call")
	}
}

func TestMultiClientUpdateState(t *testing.T) {
	scenario := testserver.NewTestScenario(t, 10, log.NewTestLogger())
	mcc, err := grpcutil.NewMultiClientConn("testconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	require.NoError(t, err)
	connMap := map[string]func() (net.Conn, error){"a mux!": scenario.Muxes[0].ClientMux.Open}
	mcc.UpdateState(connMap)
	requireCallWithNewClient(t, mcc)
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

func TestMultiClientUpdateStateCachedClients(t *testing.T) {
	scenario := testserver.NewTestScenario(t, 10, log.NewTestLogger())
	mcc, err := grpcutil.NewMultiClientConn("testconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	require.NoError(t, err)
	connMap := map[string]func() (net.Conn, error){"a mux!": scenario.Muxes[0].ClientMux.Open}
	mcc.UpdateState(connMap)
	client := adminservice.NewAdminServiceClient(mcc)
	wedge := semaphore.NewWeighted(500)
	wg := &sync.WaitGroup{}
	wg.Add(500)
	responsesCh := make(chan map[string]int)
	go func() {
		responses := make(map[string]int, 10)
		for range 1000 {
			_ = wedge.Acquire(context.Background(), 1)
			wg.Done()
			resp, err := client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
			require.NoError(t, err)
			responses[resp.ClusterName]++
		}
		responsesCh <- responses
	}()
	wg.Wait()
	mcc.UpdateState(map[string]func() (net.Conn, error){
		"mux 1": scenario.Muxes[1].ClientMux.Open,
		"mux 2": scenario.Muxes[2].ClientMux.Open,
		"mux 3": scenario.Muxes[3].ClientMux.Open,
	})
	scenario.CloseMux(0)
	scenario.CloseMux(2) // Oh no! mux 2 died!
	wg.Add(500)
	wedge.Release(500)
	responses := <-responsesCh
	require.Equal(t, responses["adminService on mux 0"], 500, responses)
	// Requests may not be even between mux 1 and 3, but they will be close
	require.True(t, responses["adminService on mux 1"] > 240, responses)
	require.Equal(t, responses["adminService on mux 2"], 0, responses)
	require.True(t, responses["adminService on mux 3"] > 240, responses)
}

func TestMultiClientWithFailedMuxes(t *testing.T) {
	scenario := testserver.NewTestScenario(t, 10, log.NewTestLogger())
	mcc, err := grpcutil.NewMultiClientConn("testconn",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`))
	require.NoError(t, err)
	mcc.UpdateState(map[string]func() (net.Conn, error){
		"mux 0": scenario.Muxes[0].ClientMux.Open,
		"mux 1": scenario.Muxes[1].ClientMux.Open,
		"mux 2": scenario.Muxes[2].ClientMux.Open,
		"mux 3": scenario.Muxes[3].ClientMux.Open,
	})
	client := adminservice.NewAdminServiceClient(mcc)
	shutdownCh := make(chan struct{})
	clientExited := make(chan struct{}, 1)
	successCh := make(chan struct{}, 100)
	errorCh := make(chan error, 100)
	go func() {
		for {
			_, err := client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
			if err != nil {
				errorCh <- err
			} else {
				successCh <- struct{}{}
			}
			select {
			case <-shutdownCh:
				clientExited <- struct{}{}
				return
			default:
			}
		}
	}()
	var successes, errors int
	for i := range 3 {
		t.Log("Closing mux", i)
		scenario.CloseMux(i)
		for range 10_000 {
			select {
			case <-successCh:
				successes++
			case err = <-errorCh:
				errors++
				t.Log("Unexpected error:", err)
			}
		}
	}
	// Don't close the last mux, we'll throw errors
	//scenario.CloseMux(3)
	shutdownCh <- struct{}{}
	<-clientExited
drainLoop:
	for {
		select {
		case err = <-errorCh:
			errors++
			t.Log("Unexpected error:", err)
		case <-successCh:
			successes++
		default:
			break drainLoop
		}
	}
	t.Log("Successes:", successes, "Errors:", errors)
	require.True(t, errors < 3, "A single error can happen per mux close, if the conn is actively transferring data")
	require.True(t, successes > 3000, "Should have reported at least 300 successes")
}

func requireCallWithNewClient(t *testing.T, mcc *grpcutil.MultiClientConn) {
	client := adminservice.NewAdminServiceClient(mcc)
	resp, err := client.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
	require.NoError(t, err)
	t.Log("Connected to cluster", resp.ClusterName)
}
