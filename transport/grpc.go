package transport

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/backoff"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/resolver"
	"google.golang.org/grpc/resolver/manual"
)

const (
	// DefaultServiceConfig is a default gRPC connection service config which enables DNS round robin between IPs.
	// To use DNS resolver, a "dns:///" prefix should be applied to the hostPort.
	// https://github.com/grpc/grpc/blob/master/doc/naming.md
	DefaultServiceConfig = `{"loadBalancingConfig": [{"round_robin":{}}]}`

	CustomServiceConfig = `{"loadBalancingConfig": [{"custom_round_robin":{}}]}`

	// MaxBackoffDelay is a maximum interval between reconnect attempts.
	MaxBackoffDelay = 10 * time.Second

	// minConnectTimeout is the minimum amount of time we are willing to give a connection to complete.
	minConnectTimeout = 20 * time.Second

	// maxInternodeRecvPayloadSize indicates the internode max receive payload size.
	maxInternodeRecvPayloadSize = 128 * 1024 * 1024 // 128 Mb
)

// Dial creates a client connection to the given target with default options.
// The hostName syntax is defined in
// https://github.com/grpc/grpc/blob/master/doc/naming.md.
// e.g. to use dns resolver, a "dns:///" prefix should be applied to the target.
func dial(hostName string, tlsConfig *tls.Config, clientMetrics *grpcprom.ClientMetrics, dialer func(ctx context.Context, addr string) (net.Conn, error)) (*grpc.ClientConn, error) {
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
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(maxInternodeRecvPayloadSize)),
		grpc.WithDisableServiceConfig(),
		grpc.WithConnectParams(cp),
		grpc.WithUnaryInterceptor(clientMetrics.UnaryClientInterceptor()),
		grpc.WithStreamInterceptor(clientMetrics.StreamClientInterceptor()),
	}

	if dialer != nil {
		dialOptions = append(dialOptions,
			grpc.WithContextDialer(dialer))
	}

	// If we are not mux transport, use a custom grpc load balancer
	// which force round robins.
	if hostName != "unused" {
		mr := manual.NewBuilderWithScheme("example")
		mr.InitialState(resolver.State{
			Endpoints: []resolver.Endpoint{
				{Addresses: []resolver.Address{{Addr: "localhost:7233"}}},
				{Addresses: []resolver.Address{{Addr: "127.0.0.1:7233"}}},
			},
			// I don't see an exported function for parsing service config.
			// This seems to re-use the default service config included in the dial options,
			// based on debug logging...
			//ServiceConfig: sc,
		})

		dialOptions = append(dialOptions,
			grpc.WithResolvers(mr),
			grpc.WithDefaultServiceConfig(CustomServiceConfig),
		)
		// nolint:staticcheck // SA1019 grpc.Dial is deprecated. Need to figure out how to use grpc.NewClient.
		return grpc.Dial(
			mr.Scheme()+":///",
			dialOptions...,
		)

	} else {
		dialOptions = append(dialOptions,
			grpc.WithDefaultServiceConfig(DefaultServiceConfig),
		)
		// nolint:staticcheck // SA1019 grpc.Dial is deprecated. Need to figure out how to use grpc.NewClient.
		return grpc.Dial(
			hostName,
			dialOptions...,
		)

	}

}
