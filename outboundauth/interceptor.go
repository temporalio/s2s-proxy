package outboundauth

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func UnaryClientInterceptor(keys []string) grpc.UnaryClientInterceptor {
	owned := toSet(keys)
	return func(
		ctx context.Context,
		method string,
		req any,
		reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {
		return invoker(stripMetadata(ctx, owned), method, req, reply, cc, opts...)
	}
}

func StreamClientInterceptor(keys []string) grpc.StreamClientInterceptor {
	owned := toSet(keys)
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		return streamer(stripMetadata(ctx, owned), desc, cc, method, opts...)
	}
}

func stripMetadata(ctx context.Context, owned map[string]struct{}) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		return ctx
	}

	clean := make(metadata.MD, len(md))
	for key, values := range md {
		if _, strip := owned[strings.ToLower(key)]; strip {
			continue
		}
		clean[key] = append([]string(nil), values...)
	}
	return metadata.NewOutgoingContext(ctx, clean)
}
