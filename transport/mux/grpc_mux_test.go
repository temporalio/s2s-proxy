package mux

import (
	"context"
	"testing"
	"time"

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
