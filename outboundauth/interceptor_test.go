package outboundauth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

func TestUnaryClientInterceptorStripsOwnedMetadata(t *testing.T) {
	ctx := metadata.NewOutgoingContext(t.Context(), metadata.MD{
		"Authorization": {"attacker-one"},
		"authorization": {"attacker-two"},
		"routing":       {"preserved"},
	})
	interceptor := UnaryClientInterceptor([]string{"authorization"})

	err := interceptor(ctx, "/service/method", nil, nil, nil, func(ctx context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		outgoing, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		assert.Empty(t, outgoing.Get("authorization"))
		assert.Equal(t, []string{"preserved"}, outgoing.Get("routing"))
		return nil
	})
	require.NoError(t, err)
}

func TestStreamClientInterceptorStripsOwnedMetadata(t *testing.T) {
	ctx := metadata.NewOutgoingContext(t.Context(), metadata.MD{
		"authorization":              {"attacker-one", "attacker-two"},
		"temporal-client-cluster-id": {"1"},
		"temporal-client-shard-id":   {"2"},
		"temporal-server-cluster-id": {"3"},
		"temporal-server-shard-id":   {"4"},
	})
	interceptor := StreamClientInterceptor([]string{"authorization"})

	_, err := interceptor(ctx, &grpc.StreamDesc{}, nil, "/service/stream", func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		outgoing, ok := metadata.FromOutgoingContext(ctx)
		require.True(t, ok)
		assert.Empty(t, outgoing.Get("authorization"))
		assert.Equal(t, []string{"1"}, outgoing.Get("temporal-client-cluster-id"))
		assert.Equal(t, []string{"2"}, outgoing.Get("temporal-client-shard-id"))
		assert.Equal(t, []string{"3"}, outgoing.Get("temporal-server-cluster-id"))
		assert.Equal(t, []string{"4"}, outgoing.Get("temporal-server-shard-id"))
		return nil, nil
	})
	require.NoError(t, err)
}
