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
// Install it where the request names the local namespace, because that is what
// the encryption config is keyed by. Which side of namespace translation that
// falls on depends on who is making the request. The local cluster already sends
// local names, while a peer cluster sends its own and needs translating first.
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
