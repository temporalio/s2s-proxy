package extension

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	kms "github.com/temporalio/temporal-proxy/pkg/api/kms/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
)

func TestDialNoServers(t *testing.T) {
	conns, err := Dial(t.Context(), nil)
	require.NoError(t, err)
	require.Empty(t, conns)
}

func TestDialReachesEveryServer(t *testing.T) {
	hsm := startKMSServer(t, "wrapped-by-hsm")
	vault := startKMSServer(t, "wrapped-by-vault")

	conns, err := Dial(t.Context(), []config.ExtensionServer{
		{Name: "hsm", Address: hsm},
		{Name: "vault", Address: vault},
	})
	require.NoError(t, err)
	require.Len(t, conns, 2)

	for name, want := range map[string]string{"hsm": "wrapped-by-hsm", "vault": "wrapped-by-vault"} {
		ct, err := NewKMS("extension://"+name+"/key", conns[name]).
			Encrypt(t.Context(), "tenant-a", []byte("dek"))
		require.NoError(t, err)
		require.Equal(t, want, string(ct))
	}
}

// TestDialSharesOneConnectionAcrossKeys covers the reason KMS.Close is a no-op:
// several keys live on one server, so closing one must leave the others working.
func TestDialSharesOneConnectionAcrossKeys(t *testing.T) {
	conns, err := Dial(t.Context(), []config.ExtensionServer{
		{Name: "hsm", Address: startKMSServer(t, "wrapped")},
	})
	require.NoError(t, err)

	first := NewKMS("extension://hsm/one", conns["hsm"])
	second := NewKMS("extension://hsm/two", conns["hsm"])
	require.NoError(t, first.Close())

	ct, err := second.Encrypt(t.Context(), "tenant-a", []byte("dek"))
	require.NoError(t, err)
	require.Equal(t, "wrapped", string(ct))
}

func TestDialClosesConnectionsWhenTheLifetimeEnds(t *testing.T) {
	lifetime, cancel := context.WithCancel(t.Context())

	conns, err := Dial(lifetime, []config.ExtensionServer{
		{Name: "hsm", Address: startKMSServer(t, "wrapped")},
	})
	require.NoError(t, err)

	cc, ok := conns["hsm"].(*grpc.ClientConn)
	require.True(t, ok)

	cancel()
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		assert.Equal(c, connectivity.Shutdown, cc.GetState())
	}, timeout, interval)
}

func TestDialRejectsDuplicateNames(t *testing.T) {
	addr := startKMSServer(t, "wrapped")

	_, err := Dial(t.Context(), []config.ExtensionServer{
		{Name: "hsm", Address: addr},
		{Name: "hsm", Address: addr},
	})
	require.ErrorContains(t, err, "hsm")
}

// TestDialClosesWhatItOpenedOnFailure asserts a failure part way through leaves
// nothing behind, rather than leaning on the caller to cancel the lifetime.
func TestDialClosesWhatItOpenedOnFailure(t *testing.T) {
	conns, err := Dial(t.Context(), []config.ExtensionServer{
		{Name: "hsm", Address: startKMSServer(t, "wrapped")},
		{
			Name:    "broken",
			Address: "127.0.0.1:9443",
			// TLS is on, but with verification enabled and no CAServerName to
			// verify against, so building the client config fails.
			TLSConfig: encryption.TLSConfig{
				CertificatePath: "/nonexistent/cert.pem",
				KeyPath:         "/nonexistent/key.pem",
			},
		},
	})
	require.Error(t, err)
	require.Nil(t, conns)
}

// TestDialOptionsReachesAStandardServer dials a plain gRPC server - one that
// knows nothing of this proxy's codec - and makes a call. That is what an
// extension server is, so these options have to work against it.
//
// Note this cannot currently tell dialOptions apart from
// grpcutil.MakeDialOptions: grpc-go still falls back to the proto codec when it
// does not recognize a content-subtype, warning that it "will start to fail in
// future releases". The test pins the behavior that matters so the fallback
// going away is a test failure and not an outage.
func TestDialOptionsReachesAStandardServer(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	healthpb.RegisterHealthServer(srv, health.NewServer())

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	cc, err := grpc.NewClient(
		lis.Addr().String(),
		dialOptions(nil, metrics.GetGRPCClientMetrics("extension"))...,
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cc.Close() })

	res, err := healthpb.NewHealthClient(cc).Check(t.Context(), &healthpb.HealthCheckRequest{})
	require.NoError(t, err)
	require.Equal(t, healthpb.HealthCheckResponse_SERVING, res.Status)
}

// startKMSServer runs a plain gRPC EncryptionService on a loopback port,
// returning its address. It is hand-rolled rather than built on temporal-proxy's
// pkg/ext, because ext.Serve installs process signal handlers and blocks, which
// a test binary has no use for.
func startKMSServer(t *testing.T, ciphertext string) string {
	t.Helper()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	kms.RegisterEncryptionServiceServer(srv, &fakeKMSServer{ciphertext: ciphertext})

	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	return lis.Addr().String()
}

type fakeKMSServer struct {
	kms.UnimplementedEncryptionServiceServer

	ciphertext string
}

func (f *fakeKMSServer) Encrypt(_ context.Context, _ *kms.EncryptRequest) (*kms.EncryptResponse, error) {
	return &kms.EncryptResponse{Ciphertext: []byte(f.ciphertext)}, nil
}
