package grpcutil

import (
	"context"
	"crypto/tls"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/proto/compat"
)

const (
	// DefaultServiceConfig is a default gRPC connection service config which enables DNS round robin between IPs.
	// To use DNS resolver, a "dns:///" prefix should be applied to the hostPort.
	// https://github.com/grpc/grpc/blob/master/doc/naming.md
	DefaultServiceConfig = `{"loadBalancingConfig": [{"round_robin":{}}]}`

	// MaxBackoffDelay is a maximum interval between reconnect attempts.
	MaxBackoffDelay = 10 * time.Second

	// MinConnectTimeout is the minimum amount of time we are willing to give a connection to complete.
	MinConnectTimeout = 20 * time.Second

	// maxInternodeRecvPayloadSize indicates the internode max receive payload size.
	maxInternodeRecvPayloadSize = 128 * 1024 * 1024 // 128 Mb
)

// ClientOptions are optional settings for MakeDialOptions.
type ClientOptions struct {
	// PerRPCCredentials, when set, are attached to every call made on the connection.
	PerRPCCredentials credentials.PerRPCCredentials
	// StripOutgoingMetadataKeys are removed from the outgoing metadata of every call made on the connection.
	// PerRPCCredentials are added after this runs, so they are never removed.
	StripOutgoingMetadataKeys []string
}

func MakeDialOptions(tlsConfig *tls.Config, clientMetrics *grpcprom.ClientMetrics, clientOptions ...ClientOptions) []grpc.DialOption {
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
		MinConnectTimeout: MinConnectTimeout,
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
	}

	var unaryInterceptors []grpc.UnaryClientInterceptor
	var streamInterceptors []grpc.StreamClientInterceptor
	for _, options := range clientOptions {
		if len(options.StripOutgoingMetadataKeys) > 0 {
			unaryInterceptors = append(unaryInterceptors, stripUnaryInterceptor(options.StripOutgoingMetadataKeys))
			streamInterceptors = append(streamInterceptors, stripStreamInterceptor(options.StripOutgoingMetadataKeys))
		}
		if options.PerRPCCredentials != nil {
			dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(options.PerRPCCredentials))
		}
	}
	unaryInterceptors = append(unaryInterceptors, clientMetrics.UnaryClientInterceptor())
	streamInterceptors = append(streamInterceptors, clientMetrics.StreamClientInterceptor())
	return append(dialOptions,
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...),
	)
}

// stripOutgoingMetadata returns ctx with keys removed from its outgoing metadata. The caller's metadata is not
// modified.
func stripOutgoingMetadata(ctx context.Context, keys []string) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return ctx
	}
	md = md.Copy()
	for _, key := range keys {
		md.Delete(key)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

func stripUnaryInterceptor(keys []string) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker, opts ...grpc.CallOption,
	) error {
		return invoker(stripOutgoingMetadata(ctx, keys), method, req, reply, cc, opts...)
	}
}

func stripStreamInterceptor(keys []string) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string,
		streamer grpc.Streamer, opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(stripOutgoingMetadata(ctx, keys), desc, cc, method, opts...)
	}
}
