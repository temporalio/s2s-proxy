package interceptor

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
)

func TestStampNamespace(t *testing.T) {
	t.Run("puts the request's namespace on the handler's context", func(t *testing.T) {
		req := &workflowservice.StartWorkflowExecutionRequest{Namespace: "tenant-a"}

		var seen any
		_, err := StampNamespace(t.Context(), req, nil, func(ctx context.Context, _ any) (any, error) {
			seen = ctx.Value(NamespaceKey)

			return nil, nil
		})

		require.NoError(t, err)
		require.Equal(t, "tenant-a", seen)
	})

	t.Run("an empty namespace leaves the context alone", func(t *testing.T) {
		// Stamping "" would satisfy the Encryptor's resolver check for a value
		// being present while telling it nothing, so leave the context bare and
		// let the resolver report the namespace as missing.
		req := &workflowservice.StartWorkflowExecutionRequest{}

		var seen any
		_, err := StampNamespace(t.Context(), req, nil, func(ctx context.Context, _ any) (any, error) {
			seen = ctx.Value(NamespaceKey)

			return nil, nil
		})

		require.NoError(t, err)
		require.Nil(t, seen)
	})

	t.Run("a request with only a namespace ID is left unstamped", func(t *testing.T) {
		// Admin replication requests carry a NamespaceId UUID and no name. The
		// encryption config is keyed by name, so there is nothing to stamp; the
		// request must still reach the handler rather than panic on its way.
		req := &adminservice.SyncWorkflowStateRequest{NamespaceId: "6f7d1b9e-0000-0000-0000-000000000000"}

		var seen any
		var reached bool
		_, err := StampNamespace(t.Context(), req, nil, func(ctx context.Context, _ any) (any, error) {
			reached = true
			seen = ctx.Value(NamespaceKey)

			return nil, nil
		})

		require.NoError(t, err)
		require.True(t, reached)
		require.Nil(t, seen)
	})

	t.Run("the handler's response and error are returned untouched", func(t *testing.T) {
		boom := errors.New("upstream is unwell")
		req := &workflowservice.StartWorkflowExecutionRequest{Namespace: "tenant-a"}

		resp, err := StampNamespace(t.Context(), req, nil, func(context.Context, any) (any, error) {
			return "the response", boom
		})

		require.Equal(t, "the response", resp)
		require.ErrorIs(t, err, boom)
	})
}

func TestStampNamespaceFeedsTheEncryptor(t *testing.T) {
	v := &fakeVault{}
	e := requireEncryptor(t, enabledConfig(v))

	req := &workflowservice.StartWorkflowExecutionRequest{
		Namespace: "tenant-a",
		Input:     payloads("secret"),
	}

	_, err := StampNamespace(t.Context(), req, nil, func(ctx context.Context, r any) (any, error) {
		return nil, callWith(t, ctx, e, r, new(workflowservice.QueryWorkflowResponse), func() error {
			return nil
		})
	})

	require.NoError(t, err)
	require.Equal(t, []string{"tenant-a"}, v.namespaces())
}
