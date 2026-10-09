package grpcutil

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/temporalio/s2s-proxy/metrics"
)

// TestMakeExtensionDialOptionsReachesAStandardServer dials a plain gRPC server -
// one that knows nothing of this proxy's codec - and makes a call. That is what
// an extension server is, so these options have to work against it.
//
// Note this cannot currently tell the two helpers apart: grpc-go still falls
// back to the proto codec when it does not recognize a content-subtype, warning
// that it "will start to fail in future releases". The test pins the behavior
// that matters so the fallback going away is a test failure and not an outage.
func TestMakeExtensionDialOptionsReachesAStandardServer(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	healthpb.RegisterHealthServer(srv, health.NewServer())

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	cc, err := grpc.NewClient(
		lis.Addr().String(),
		MakeExtensionDialOptions(nil, metrics.GetGRPCClientMetrics("extension"))...,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })

	res, err := healthpb.NewHealthClient(cc).Check(context.Background(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, res.Status)
}
