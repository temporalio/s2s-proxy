package interceptor

import (
	"context"

	"google.golang.org/grpc"
)

// namespaceKey keys the request namespace on a context. The type is unexported
// so nothing outside this package can construct one, which means nothing else
// can plant a namespace for [StampNamespace]'s readers to find.
type namespaceKey struct{}

// NamespaceKey is where [StampNamespace] leaves the namespace and where
// everything downstream looks for it, [visitPayloads] above all.
var NamespaceKey namespaceKey

// StampNamespace is a [grpc.UnaryServerInterceptor] that records the namespace
// a request names, so later stages can act on it without re-reading the
// message. It reads the request's own top-level namespace and nothing else: a
// namespace mentioned inside a history event belongs to that event, not to the
// request carrying it.
//
// Install it before anything that rewrites namespaces, so what it records is
// the name the caller actually sent. That is what the encryption config is
// keyed by, and it is the whole point of recording the value here rather than
// letting a later stage read whatever the message says by then.
//
// A request that names no namespace leaves the context untouched rather than
// stamping "". Requests whose namespace is only a NamespaceId are in that group:
// the ID is not a name, and nothing here maps one to the other. Callers that
// need a namespace are expected to fail when it is absent rather than fall back
// to a default, so an unstamped context has to stay distinguishable from one
// that named something.
func StampNamespace(
	ctx context.Context,
	req any,
	_ *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if named, ok := req.(interface{ GetNamespace() string }); ok {
		if ns := named.GetNamespace(); ns != "" {
			ctx = context.WithValue(ctx, NamespaceKey, ns)
		}
	}

	return handler(ctx, req)
}
