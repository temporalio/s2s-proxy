package grpcutil

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

func TestStripOutgoingMetadata(t *testing.T) {
	original := metadata.Pairs(
		"authorization", "Bearer peer",
		"Authorization-Extras", "extras",
		"temporal-client-shard-id", "4",
	)
	ctx := metadata.NewOutgoingContext(t.Context(), original)

	stripped, ok := metadata.FromOutgoingContext(stripOutgoingMetadata(ctx, []string{"authorization", "authorization-extras"}))
	require.True(t, ok)
	require.Equal(t, metadata.Pairs("temporal-client-shard-id", "4"), stripped)

	// The caller's metadata is left alone.
	require.Equal(t, []string{"Bearer peer"}, original.Get("authorization"))
	require.Equal(t, []string{"extras"}, original.Get("authorization-extras"))
}

func TestStripOutgoingMetadataWithoutMetadata(t *testing.T) {
	ctx := t.Context()
	require.Equal(t, ctx, stripOutgoingMetadata(ctx, []string{"authorization"}))
}
