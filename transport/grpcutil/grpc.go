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

	"github.com/temporalio/s2s-proxy/outboundauth"
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
)

type ClientOptions struct {
	PerRPCCredentials         credentials.PerRPCCredentials
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
		MinConnectTimeout: minConnectTimeout,
	}
	cp.Backoff.MaxDelay = MaxBackoffDelay

	var options ClientOptions
	if len(clientOptions) > 0 {
		options = clientOptions[0]
	}

	unaryInterceptors := make([]grpc.UnaryClientInterceptor, 0, 2)
	streamInterceptors := make([]grpc.StreamClientInterceptor, 0, 2)
	if len(options.StripOutgoingMetadataKeys) > 0 {
		unaryInterceptors = append(unaryInterceptors, outboundauth.UnaryClientInterceptor(options.StripOutgoingMetadataKeys))
		streamInterceptors = append(streamInterceptors, outboundauth.StreamClientInterceptor(options.StripOutgoingMetadataKeys))
	}
	unaryInterceptors = append(unaryInterceptors, clientMetrics.UnaryClientInterceptor())
	streamInterceptors = append(streamInterceptors, clientMetrics.StreamClientInterceptor())

	dialOptions := []grpc.DialOption{
		grpcSecureOpt,
		grpc.WithDefaultCallOptions(
			grpc.ForceCodecV2(encoding.GetCodecV2(compat.CodecName)),
			grpc.MaxCallRecvMsgSize(maxInternodeRecvPayloadSize),
		),
		grpc.WithDefaultServiceConfig(DefaultServiceConfig),
		grpc.WithDisableServiceConfig(),
		grpc.WithConnectParams(cp),
		grpc.WithChainUnaryInterceptor(unaryInterceptors...),
		grpc.WithChainStreamInterceptor(streamInterceptors...),
	}
	if options.PerRPCCredentials != nil {
		dialOptions = append(dialOptions, grpc.WithPerRPCCredentials(options.PerRPCCredentials))
	}
	return dialOptions
}
