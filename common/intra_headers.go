package common

import (
	"context"

	"google.golang.org/grpc/metadata"
)

const (
	// Intra-proxy identification and tracing headers
	IntraProxyHeaderKey           = "x-s2s-intra-proxy"
	IntraProxyHeaderValue         = "1"
	IntraProxyOriginProxyIDHeader = "x-s2s-origin-proxy-id"
	IntraProxyHopCountHeader      = "x-s2s-hop-count"
	IntraProxyTraceIDHeader       = "x-s2s-trace-id"
)

// IsIntraProxy checks incoming context metadata for intra-proxy marker.
func IsIntraProxy(ctx context.Context) bool {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if vals := md.Get(IntraProxyHeaderKey); len(vals) > 0 && vals[0] == IntraProxyHeaderValue {
			return true
		}
	}
	return false
}

// WithIntraProxyHeaders returns a new outgoing context with intra-proxy headers set.
func WithIntraProxyHeaders(ctx context.Context, headers map[string]string) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(IntraProxyHeaderKey, IntraProxyHeaderValue)
	for k, v := range headers {
		md.Set(k, v)
	}
	return metadata.NewOutgoingContext(ctx, md)
}
