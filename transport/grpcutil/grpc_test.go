package grpcutil

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"sync/atomic"
	"testing"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type rotatingTestCredentials struct {
	calls atomic.Int32
}

func (c *rotatingTestCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	c.calls.Add(1)
	return map[string]string{"x-custom-auth": "provider-value"}, nil
}

func (*rotatingTestCredentials) RequireTransportSecurity() bool {
	return true
}

func TestMakeDialOptionsAppliesCredentialsToUnaryAndStreamRPCs(t *testing.T) {
	serverTLS, clientTLS := testTLSConfigs(t)
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(serverTLS)),
		grpc.ChainUnaryInterceptor(requireCredentialUnary),
		grpc.ChainStreamInterceptor(requireCredentialStream),
	)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	go func() {
		_ = server.Serve(listener)
	}()
	t.Cleanup(server.Stop)

	callCredentials := &rotatingTestCredentials{}
	options := MakeDialOptions(
		clientTLS,
		grpcprom.NewClientMetrics(),
		ClientOptions{
			PerRPCCredentials:         callCredentials,
			StripOutgoingMetadataKeys: []string{"x-custom-auth"},
		},
	)
	options = append(options, grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
		return listener.Dial()
	}))
	clientConnection, err := grpc.NewClient("passthrough:///bufnet", options...)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, clientConnection.Close()) })
	client := grpc_health_v1.NewHealthClient(clientConnection)
	ctx := metadata.NewOutgoingContext(t.Context(), metadata.MD{
		"X-Custom-Auth":            {"attacker-one"},
		"x-custom-auth":            {"attacker-two"},
		"temporal-client-shard-id": {"12"},
	})

	response, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, response.Status)

	streamCtx, cancel := context.WithCancel(ctx)
	stream, err := client.Watch(streamCtx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = stream.Recv()
	require.NoError(t, err)
	cancel()

	secondStreamCtx, secondCancel := context.WithCancel(ctx)
	secondStream, err := client.Watch(secondStreamCtx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	_, err = secondStream.Recv()
	require.NoError(t, err)
	secondCancel()
	assert.Equal(t, int32(3), callCredentials.calls.Load())
}

func requireCredential(ctx context.Context) error {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return status.Error(codes.Unauthenticated, "missing metadata")
	}
	values := md.Get("x-custom-auth")
	if len(values) != 1 || values[0] != "provider-value" {
		return status.Error(codes.Unauthenticated, "invalid credential")
	}
	if values := md.Get("temporal-client-shard-id"); len(values) != 1 || values[0] != "12" {
		return status.Error(codes.InvalidArgument, "missing routing metadata")
	}
	return nil
}

func requireCredentialUnary(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if err := requireCredential(ctx); err != nil {
		return nil, err
	}
	return handler(ctx, req)
}

func requireCredentialStream(srv any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	if err := requireCredential(stream.Context()); err != nil {
		return err
	}
	return handler(srv, stream)
}

func testTLSConfigs(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		DNSNames:     []string{"localhost"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, publicKey, privateKey)
	require.NoError(t, err)
	certificate, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	pool := x509.NewCertPool()
	pool.AddCert(certificate)
	serverTLS := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: privateKey}},
	}
	clientTLS := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    pool,
		ServerName: "localhost",
	}
	return serverTLS, clientTLS
}
