package mux

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/testserver"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

func TestGRPCMux(t *testing.T) {
	logger := log.NewTestLogger()

	// Receiving
	serverConfig := config.MuxTransportConfig{
		Name: "serverMux",
		Mode: config.ServerMode,
		Server: config.TCPServerSetting{
			ListenAddress: "127.0.0.1:0",
			// No TLS
			// No external address
		},
	}
	receivingClient, err := grpcutil.NewMultiClientConn("receivingClientConns", grpcutil.MakeDialOptions(nil, metrics.GRPCOutboundClientMetrics)...)
	receivingServerDefn := grpc.NewServer()
	receivingEas := &testserver.EchoAdminService{
		ServiceName: "receivingServerDefn",
		Logger:      log.With(logger, common.ServiceTag("receivingServerDefn"), tag.Address("127.0.0.1:0")),
		Namespaces:  map[string]bool{"hello": true, "world": true},
		PayloadSize: 1024,
	}
	adminservice.RegisterAdminServiceServer(receivingServerDefn, receivingEas)
	require.NoError(t, err)
	receivingMuxManager, err := NewGRPCMuxManager(t.Context(), serverConfig, receivingClient, receivingServerDefn, logger)
	require.NoError(t, err)
	receivingMuxManager.Start()
	receivingAdminServiceClient := adminservice.NewAdminServiceClient(receivingClient)
	reqSuccess := make(chan struct{}, 1)
	go func() {
		receivingAdminServiceClient.DescribeCluster(context.Background(), &adminservice.DescribeClusterRequest{})
		<-reqSuccess
	}()
	requireNoCh(t, reqSuccess, time.Millisecond*20, "Should not have completed request")
	// TODO: Confirm no connections available from receivingClient

	// Establishing
	clientConfig := config.MuxTransportConfig{
		Name: "clientMux",
		Mode: config.ClientMode,
		Client: config.TCPClientSetting{
			ServerAddress: receivingMuxManager.Address(),
			// No TLS
		},
	}
	establishingClient, err := grpcutil.NewMultiClientConn("establishingClientConns", grpcutil.MakeDialOptions(nil, metrics.GRPCOutboundClientMetrics)...)
	require.NoError(t, err)
	establishingServer := grpc.NewServer()
	establishingEas := &testserver.EchoAdminService{
		ServiceName: "establishingServer",
		Logger:      log.With(logger, common.ServiceTag("receivingServerDefn"), tag.Address("127.0.0.1:0")),
		Namespaces:  map[string]bool{"hello": true, "world": true},
		PayloadSize: 1024,
	}
	adminservice.RegisterAdminServiceServer(establishingServer, establishingEas)
	establishingMuxManager, err := NewGRPCMuxManager(t.Context(), clientConfig, establishingClient, establishingServer, logger)
	require.NoError(t, err)
	establishingMuxManager.Start()
}
