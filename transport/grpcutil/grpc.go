package grpcutil

import (
	"crypto/tls"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"

	"github.com/temporalio/s2s-proxy/proto/compat"
)

const (
	// DefaultServiceConfig is a default gRPC connection service config which enables DNS round robin between IPs.
	// To use DNS resolver, a "dns:///" prefix should be applied to the hostPort.
	// https://github.com/grpc/grpc/blob/master/doc/naming.md
	DefaultServiceConfig = `{"loadBalancingConfig": [{"round_robin":{}}]}`

	// MaxBackoffDelay is a maximum interval between reconnect attempts.
	MaxBackoffDelay = 10 * time.Second

	// minConnectTimeout is the minimum amount of time we are willing to give a connection to complete.
	minConnectTimeout = 20 * time.Second

	// maxInternodeRecvPayloadSize indicates the internode max receive payload size.
	maxInternodeRecvPayloadSize = 128 * 1024 * 1024 // 128 Mb

	// maxExtensionRecvPayloadSize bounds what an extension server may return. It
	// is sized for key material rather than replication payloads, and matches the
	// limit temporal-proxy's pkg/ext applies on the serving side.
	maxExtensionRecvPayloadSize = 1024 * 1024 // 1 Mb
)

func MakeDialOptions(tlsConfig *tls.Config, clientMetrics *grpcprom.ClientMetrics) []grpc.DialOption {
	var grpcSecureOpt grpc.DialOption
	if tlsConfig == nil {
		grpcSecureOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	} else {
		grpcSecureOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	}

	// gRPC maintains connection pool inside grpc.ClientConn.
	// This connection pool has auto reconnect feature.
	// If connection goes down, gRPC will try to reconnect using exponential backoff strategy:
	// https://github.com/grpc/grpc/blob/master/doc/connection-backoff.md.
	// Default MaxDelay is 120 seconds which is too high.
	var cp = grpc.ConnectParams{
		Backoff:           backoff.DefaultConfig,
		MinConnectTimeout: minConnectTimeout,
	}
	cp.Backoff.MaxDelay = MaxBackoffDelay

	dialOptions := []grpc.DialOption{
		grpcSecureOpt,
		grpc.WithDefaultCallOptions(
			grpc.ForceCodecV2(encoding.GetCodecV2(compat.CodecName)),
			grpc.MaxCallRecvMsgSize(maxInternodeRecvPayloadSize),
		),
		grpc.WithDefaultServiceConfig(DefaultServiceConfig),
		grpc.WithDisableServiceConfig(),
		grpc.WithConnectParams(cp),
		grpc.WithUnaryInterceptor(clientMetrics.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(clientMetrics.StreamClientInterceptor()),
	}
	return dialOptions
}

// MakeExtensionDialOptions builds the dial options for an extension server: an
// operator-run gRPC service the proxy calls to wrap and unwrap DEKs.
//
// It exists rather than reusing [MakeDialOptions] because that one forces this
// proxy's own codec via grpc.ForceCodecV2, setting a content-subtype an
// extension server has never heard of. grpc-go currently tolerates the mismatch
// by falling back to the proto codec, but warns while doing it:
//
//	Unsupported codec %q. Defaulting to %q for now. This will start to fail in
//	future releases.
//
// So reusing it would log on every call an operator's server serves, and break
// outright on a future grpc-go bump. The other differences follow from the same
// point: an extension server is not a Temporal node, so the 128 MiB receive
// limit and the round-robin service config do not apply to it. The connection
// backoff is shared, because wanting a reconnect sooner than two minutes is not
// specific to Temporal.
func MakeExtensionDialOptions(tlsConfig *tls.Config, clientMetrics *grpcprom.ClientMetrics) []grpc.DialOption {
	var grpcSecureOpt grpc.DialOption
	if tlsConfig == nil {
		grpcSecureOpt = grpc.WithTransportCredentials(insecure.NewCredentials())
	} else {
		grpcSecureOpt = grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig))
	}

	var cp = grpc.ConnectParams{
		Backoff:           backoff.DefaultConfig,
		MinConnectTimeout: minConnectTimeout,
	}
	cp.Backoff.MaxDelay = MaxBackoffDelay

	return []grpc.DialOption{
		grpcSecureOpt,
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(maxExtensionRecvPayloadSize),
		),
		grpc.WithConnectParams(cp),
		grpc.WithUnaryInterceptor(clientMetrics.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(clientMetrics.StreamClientInterceptor()),
	}
}
