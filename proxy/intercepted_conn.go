package proxy

import (
	"context"

	"google.golang.org/grpc"
)

// interceptedConn runs a unary client interceptor ahead of every Invoke on the
// conn it embeds. It lets one service's client be intercepted while the others
// built on the same conn are not, which a dial option cannot do.
//
// Streams pass straight through. Namespaces are only stamped on unary calls (see
// stampAndTranslate), so there is nothing for a stream interceptor to key on.
type interceptedConn struct {
	grpc.ClientConnInterface
	intercept grpc.UnaryClientInterceptor
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
