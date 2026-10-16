package interceptor

import (
	"context"
	"encoding/json"

	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/proxy"
	"go.temporal.io/proxy/pkg/codec"
	"google.golang.org/grpc"
)

// encodedFailureMarker is the metadata key encodeFailure sets on the attributes
// it creates. It is sealed along with them, so the peer never sees it, and it is
// how restoreFailure tells our encoding from the SDK's: once opened, both are
// the same json/plain payload.
const encodedFailureMarker = "s2s-proxy-encoded-failure"

// encodeFailure moves f's message and stack trace into its encoded attributes,
// through [codec.EncodeFailure], and marks them as ours so a payload codec run
// afterwards seals the text and restoreFailure can put it back. A failure that
// already has encoded attributes, typically one the SDK encoded, is left alone
// and unmarked. f's cause is not visited.
func encodeFailure(f *failure.Failure) error {
	if f.GetEncodedAttributes() != nil {
		return nil
	}

	if err := codec.EncodeFailure(f); err != nil {
		return err
	}

	f.EncodedAttributes.Metadata[encodedFailureMarker] = []byte("true")
	return nil
}

// restoreFailure undoes encodeFailure once a payload codec has opened f's
// attributes: the text goes back onto f and the attributes are dropped, so f is
// exactly what it was before it crossed. Unlike [codec.RestoreFailure] it only
// touches failures carrying our marker; anything else, the SDK's encoding above
// all, is the customer's and already matches the source.
//
// It never fails. Attributes still sealed, or marked but unreadable, keep the
// placeholder: failing the call would cost the response for the sake of a
// message.
func restoreFailure(f *failure.Failure) error {
	attrs := f.GetEncodedAttributes()
	md := attrs.GetMetadata()
	if len(md[encodedFailureMarker]) == 0 || string(md[codec.MetadataEncoding]) != "json/plain" {
		return nil
	}

	// The shape codec.EncodeFailure writes. codec.RestoreFailure cannot be used:
	// it reports unreadable attributes as success, and clearing them then would
	// lose the text.
	var text struct {
		Message    string `json:"message"`
		StackTrace string `json:"stack_trace"`
	}
	if err := json.Unmarshal(attrs.GetData(), &text); err != nil {
		return nil //nolint:nilerr // unreadable attributes keep the placeholder; see above
	}

	f.Message = text.Message
	f.StackTrace = text.StackTrace
	f.EncodedAttributes = nil

	return nil
}

// visitFailureOpts returns the options that apply fn to every failure a visit
// reaches, causes included.
func visitFailureOpts(fn func(*failure.Failure) error) *proxy.VisitFailuresOptions {
	return &proxy.VisitFailuresOptions{
		Visitor: func(_ *proxy.VisitFailuresContext, f *failure.Failure) error { return fn(f) },
	}
}

// chainUnary runs outer around inner: outer sees the request first and the
// response last.
func chainUnary(outer, inner grpc.UnaryClientInterceptor) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return outer(ctx, method, req, reply, cc,
			func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, opts ...grpc.CallOption) error {
				return inner(ctx, method, req, reply, cc, invoker, opts...)
			},
			opts...,
		)
	}
}
