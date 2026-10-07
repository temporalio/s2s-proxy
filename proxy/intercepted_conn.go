package proxy

import (
	"context"

	"google.golang.org/grpc"
)

// interceptedConn runs client interceptors ahead of the calls on the conn it
// embeds: intercept around every Invoke, and interceptStream, when set, around
// every NewStream. It lets one service's client be intercepted while the others
// built on the same conn are not, which a dial option cannot do.
type interceptedConn struct {
	grpc.ClientConnInterface
	intercept       grpc.UnaryClientInterceptor
	interceptStream grpc.StreamClientInterceptor
}

// Invoke hands the call to the interceptor, with the embedded conn as its
// invoker. The interceptor is given no *grpc.ClientConn because there may not
// be one (a mux client is not one), so it must only pass it on to the invoker.
func (c interceptedConn) Invoke(ctx context.Context, method string, req, reply any, opts ...grpc.CallOption) error {
	invoke := func(ctx context.Context, method string, req, reply any, _ *grpc.ClientConn, opts ...grpc.CallOption) error {
		return c.ClientConnInterface.Invoke(ctx, method, req, reply, opts...)
	}

	return c.intercept(ctx, method, req, reply, nil, invoke, opts...)
}

// NewStream hands the stream to interceptStream, with the embedded conn as its
// streamer, or opens it directly when there is none. As with Invoke, the
// interceptor gets no *grpc.ClientConn.
func (c interceptedConn) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	if c.interceptStream == nil {
		return c.ClientConnInterface.NewStream(ctx, desc, method, opts...)
	}

	streamer := func(ctx context.Context, desc *grpc.StreamDesc, _ *grpc.ClientConn, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		return c.ClientConnInterface.NewStream(ctx, desc, method, opts...)
	}

	return c.interceptStream(ctx, desc, nil, method, streamer, opts...)
}
