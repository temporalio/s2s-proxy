package interceptor

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/proxy"
	"go.temporal.io/proxy/pkg/codec"
	"google.golang.org/protobuf/proto"
)

func TestFailureEncoding(t *testing.T) {
	t.Run("encode then restore gives back the original", func(t *testing.T) {
		want := appFailure("boom", "at main.go:1")
		f := proto.Clone(want).(*failure.Failure)

		require.NoError(t, encodeFailure(f))
		require.Equal(t, "Encoded failure", f.GetMessage())
		require.Empty(t, f.GetStackTrace())
		require.Equal(t, "true", string(f.GetEncodedAttributes().GetMetadata()[encodedFailureMarker]))

		require.NoError(t, restoreFailure(f))
		require.True(t, proto.Equal(want, f), "got %v", f)
	})

	t.Run("causes are visited", func(t *testing.T) {
		want := appFailure("outer", "outer stack")
		want.Cause = appFailure("inner", "inner stack")
		f := proto.Clone(want).(*failure.Failure)

		require.NoError(t, proxy.VisitFailures(t.Context(), f, *visitFailureOpts(encodeFailure)))
		require.Equal(t, "Encoded failure", f.GetMessage())
		require.Equal(t, "Encoded failure", f.GetCause().GetMessage())

		require.NoError(t, proxy.VisitFailures(t.Context(), f, *visitFailureOpts(restoreFailure)))
		require.True(t, proto.Equal(want, f), "got %v", f)
	})

	t.Run("an empty failure round trips", func(t *testing.T) {
		f := &failure.Failure{}

		require.NoError(t, encodeFailure(f))
		require.NoError(t, restoreFailure(f))
		require.True(t, proto.Equal(&failure.Failure{}, f), "got %v", f)
	})

	t.Run("an SDK-encoded failure is left alone", func(t *testing.T) {
		want := sdkEncodedFailure()
		f := proto.Clone(want).(*failure.Failure)

		require.NoError(t, encodeFailure(f))
		require.True(t, proto.Equal(want, f), "encode changed it: %v", f)

		require.NoError(t, restoreFailure(f))
		require.True(t, proto.Equal(want, f), "restore changed it: %v", f)
	})

	t.Run("unreadable marked attributes keep the placeholder", func(t *testing.T) {
		f := appFailure("boom", "")
		require.NoError(t, encodeFailure(f))
		f.EncodedAttributes.Data = []byte("{not json")
		want := proto.Clone(f)

		require.NoError(t, restoreFailure(f))
		require.True(t, proto.Equal(want, f), "got %v", f)
	})

	t.Run("attributes still sealed are left alone", func(t *testing.T) {
		f := sealedFailure(t, appFailure("boom", "stack"))
		want := proto.Clone(f)

		require.NoError(t, restoreFailure(f))
		require.True(t, proto.Equal(want, f), "got %v", f)
	})
}

// appFailure is an application failure carrying msg and stack as plaintext, the
// way a worker reports one with failure encoding off.
func appFailure(msg, stack string) *failure.Failure {
	return &failure.Failure{
		Message:    msg,
		StackTrace: stack,
		Source:     "GoSDK",
		FailureInfo: &failure.Failure_ApplicationFailureInfo{
			ApplicationFailureInfo: &failure.ApplicationFailureInfo{Type: "Boom"},
		},
	}
}

// sdkEncodedFailure is a failure the way the Temporal SDK leaves it with
// EncodeCommonAttributes on: the placeholder message, and the real text in
// unmarked json/plain attributes.
func sdkEncodedFailure() *failure.Failure {
	return &failure.Failure{
		Message: "Encoded failure",
		EncodedAttributes: &common.Payload{
			Metadata: map[string][]byte{codec.MetadataEncoding: []byte("json/plain")},
			Data:     []byte(`{"message":"sdk secret","stack_trace":"sdk stack"}`),
		},
		FailureInfo: &failure.Failure_ApplicationFailureInfo{
			ApplicationFailureInfo: &failure.ApplicationFailureInfo{Type: "Boom"},
		},
	}
}

// sealedFailure returns f the way the peer holds it: encoded, causes included,
// and every payload sealed through a throwaway vault, as [sealed] does for
// payloads.
func sealedFailure(t *testing.T, f *failure.Failure) *failure.Failure {
	t.Helper()

	f = proto.Clone(f).(*failure.Failure)
	require.NoError(t, proxy.VisitFailures(t.Context(), f, *visitFailureOpts(encodeFailure)))

	chain := codec.NewChain(codec.WithCipher(&cipher{ctx: t.Context(), ns: testNamespace, v: &fakeVault{}}))
	require.NoError(t, proxy.VisitPayloads(t.Context(), f, proxy.VisitPayloadsOptions{
		Visitor: func(_ *proxy.VisitPayloadsContext, ps []*common.Payload) ([]*common.Payload, error) {
			return chain.Encode(ps)
		},
	}))

	return f
}

// requireFailureSealed checks f is what the peer should see: the placeholder,
// no stack trace, and attributes sealed with neither the text nor our marker
// readable.
func requireFailureSealed(t *testing.T, f *failure.Failure) {
	t.Helper()

	require.Equal(t, "Encoded failure", f.GetMessage())
	require.Empty(t, f.GetStackTrace())

	attrs := f.GetEncodedAttributes()
	require.Equal(t, codec.EncryptionEncoding, string(attrs.GetMetadata()[codec.MetadataEncoding]))
	require.NotContains(t, string(attrs.GetData()), "secret")
	require.NotContains(t, attrs.GetMetadata(), encodedFailureMarker)
}
