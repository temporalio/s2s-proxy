package mux

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/endtoendtest/proxyassert"
	"github.com/temporalio/s2s-proxy/endtoendtest/testservices"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
)

func init() {
	MuxManagerStartDelay = 0
}

func TestGRPCMux(t *testing.T) {
	logger := log.NewTestLogger()

	// Receiving
	serverConfig := config.ClusterDefinition{
		ConnectionType: config.ConnTypeMuxServer,
		MuxAddressInfo: config.TCPTLSInfo{
			ConnectionString: "127.0.0.1:0",
		},
	}
	receivingClient, err := grpcutil.NewMultiClientConn(t.Context(), "receivingClientConns", grpcutil.MakeDialOptions(nil, metrics.GRPCOutboundClientMetrics)...)
	receivingServerDefn := grpc.NewServer()
	receivingEas := &testservices.EchoAdminService{
		ServiceName: "receivingServerDefn",
		Logger:      log.With(logger, common.ServiceTag("receivingServerDefn"), tag.Address("127.0.0.1:0")),
		Namespaces:  map[string]bool{"hello": true, "world": true},
		PayloadSize: 1024,
	}
	adminservice.RegisterAdminServiceServer(receivingServerDefn, receivingEas)
	require.NoError(t, err)
	receivingMuxManager, err := NewGRPCMuxManager(t.Context(), "receivingMM", serverConfig, receivingClient, receivingServerDefn, logger)
	require.NoError(t, err)
	receivingMuxManager.Start()
	receivingAdminServiceClient := adminservice.NewAdminServiceClient(receivingClient)
	reqSuccess := make(chan struct{}, 1)
	go func() {
		_, _ = receivingAdminServiceClient.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		<-reqSuccess
	}()
	proxyassert.RequireNoCh(t, reqSuccess, time.Millisecond*20, "Should not have completed request")
	// TODO: Confirm no connections available from receivingClient

	// Establishing
	clientConfig := config.ClusterDefinition{
		ConnectionType: config.ConnTypeMuxClient,
		MuxAddressInfo: config.TCPTLSInfo{
			ConnectionString: "127.0.0.1:0",
		},
	}
	establishingClient, err := grpcutil.NewMultiClientConn(t.Context(), "establishingClientConns", grpcutil.MakeDialOptions(nil, metrics.GRPCOutboundClientMetrics)...)
	require.NoError(t, err)
	establishingServer := grpc.NewServer()
	establishingEas := &testservices.EchoAdminService{
		ServiceName: "establishingServer",
		Logger:      log.With(logger, common.ServiceTag("receivingServerDefn"), tag.Address("127.0.0.1:0")),
		Namespaces:  map[string]bool{"hello": true, "world": true},
		PayloadSize: 1024,
	}
	adminservice.RegisterAdminServiceServer(establishingServer, establishingEas)
	establishingMuxManager, err := NewGRPCMuxManager(t.Context(), "establishingMM", clientConfig, establishingClient, establishingServer, logger)
	require.NoError(t, err)
	establishingMuxManager.Start()
}

func TestGRPCMuxReconnectsAfterReceiverRestart(t *testing.T) {
	logger := log.NewTestLogger()
	clientLifetime, cancelClient := context.WithCancel(t.Context())
	defer cancelClient()

	establishingClient, err := grpcutil.NewMultiClientConn(clientLifetime, "reconnectingClientConns",
		grpcutil.MakeDialOptions(nil, metrics.GRPCOutboundClientMetrics)...)
	require.NoError(t, err)
	establishingServer := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(establishingServer, &testservices.EchoAdminService{
		ServiceName: "establishingServer",
		Logger:      logger,
	})

	receiverLifetime, cancelReceiver := context.WithCancel(t.Context())
	receivingClient, receivingManager := startTestMuxReceiver(t, receiverLifetime, "receiver-before-restart", "127.0.0.1:0", logger)
	establishingManager, err := NewGRPCMuxManager(clientLifetime, "reconnecting-client", config.ClusterDefinition{
		ConnectionType: config.ConnTypeMuxClient,
		MuxCount:       1,
		MuxAddressInfo: config.TCPTLSInfo{ConnectionString: receivingManager.Address()},
	}, establishingClient, establishingServer, logger)
	require.NoError(t, err)
	establishingManager.Start()

	requireBidirectionalMuxCalls(t, receivingClient, establishingClient)

	receiverAddress := receivingManager.Address()
	cancelReceiver()
	require.Eventually(t, receivingManager.IsClosed, 5*time.Second, 10*time.Millisecond)

	restartedLifetime, cancelRestarted := context.WithCancel(t.Context())
	defer cancelRestarted()
	restartedReceivingClient, restartedManager := startTestMuxReceiver(t, restartedLifetime, "receiver-after-restart", receiverAddress, logger)
	requireBidirectionalMuxCalls(t, restartedReceivingClient, establishingClient)
	require.Eventually(t, func() bool {
		return len(establishingManager.GetMuxConnections()) == 1 && len(restartedManager.GetMuxConnections()) == 1
	}, 5*time.Second, 10*time.Millisecond)
}

func TestGRPCServerRecyclesFailedMuxListenerWithoutRetrying(t *testing.T) {
	wantErr := errors.New("listener failed")
	listener := &failingListener{err: wantErr}
	serveGRPCOnMux(t.Context(), "test", "mux-1", grpc.NewServer(), listener,
		[]string{"test-address", "test-mode", "test-name"}, log.NewTestLogger())
	require.Equal(t, 1, listener.accepts)
	require.Positive(t, listener.closes)
}

type failingListener struct {
	err     error
	accepts int
	closes  int
}

func (l *failingListener) Accept() (net.Conn, error) {
	l.accepts++
	return nil, l.err
}

func (l *failingListener) Close() error {
	l.closes++
	return nil
}

func (l *failingListener) Addr() net.Addr { return testAddr("failed-mux") }

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return string(a) }

func startTestMuxReceiver(
	t *testing.T,
	lifetime context.Context,
	name string,
	address string,
	logger log.Logger,
) (*grpcutil.MultiClientConn, MultiMuxManager) {
	t.Helper()
	client, err := grpcutil.NewMultiClientConn(lifetime, name+"-client",
		grpcutil.MakeDialOptions(nil, metrics.GRPCOutboundClientMetrics)...)
	require.NoError(t, err)
	server := grpc.NewServer()
	adminservice.RegisterAdminServiceServer(server, &testservices.EchoAdminService{
		ServiceName: name,
		Logger:      logger,
	})
	manager, err := NewGRPCMuxManager(lifetime, name, config.ClusterDefinition{
		ConnectionType: config.ConnTypeMuxServer,
		MuxCount:       1,
		MuxAddressInfo: config.TCPTLSInfo{ConnectionString: address},
	}, client, server, logger)
	require.NoError(t, err)
	manager.Start()
	return client, manager
}

func requireBidirectionalMuxCalls(t *testing.T, receivingClient, establishingClient *grpcutil.MultiClientConn) {
	t.Helper()
	for name, connection := range map[string]*grpcutil.MultiClientConn{
		"receiver-to-establisher": receivingClient,
		"establisher-to-receiver": establishingClient,
	} {
		client := adminservice.NewAdminServiceClient(connection)
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			ctx, cancel := context.WithTimeout(t.Context(), 250*time.Millisecond)
			defer cancel()
			_, callErr := client.DescribeCluster(ctx, &adminservice.DescribeClusterRequest{})
			assert.NoError(collect, callErr, name)
		}, 5*time.Second, 10*time.Millisecond)
	}
}
