package interceptor

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/errordetails/v1"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/api/proxy"
	"go.temporal.io/api/workflow/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/proxy/pkg/codec"
	"go.temporal.io/proxy/pkg/crypto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

const testNamespace = "some-namespace"

type (
	// fakeVault stands in for a key-management backend. scramble is its own
	// inverse, so whatever it seals opens back to exactly what went in while never
	// being equal to it, which is enough to tell a sealed payload from a plaintext
	// one without any real keys.
	//
	// Visits run concurrently (see [visitPayloads]), so the bookkeeping is guarded.
	fakeVault struct {
		mu      sync.Mutex
		sealed  []string // namespace per Seal, in call order
		opened  []string // KEK ID per Open, in call order
		sealErr error
		openErr error
	}
)

func TestNewEncryptor(t *testing.T) {
	t.Run("encryption enabled without a vault is refused", func(t *testing.T) {
		e, err := NewEncryptor(EncryptorConfig{Enabled: true})
		require.Nil(t, e)
		require.ErrorContains(t, err, "encryption requires a vault")
	})

	t.Run("encryption disabled needs no vault", func(t *testing.T) {
		requireEncryptor(t, EncryptorConfig{})
	})

	t.Run("a vault without encryption is allowed", func(t *testing.T) {
		v := &fakeVault{}
		requireEncryptor(t, EncryptorConfig{Vault: v})

		seals, opens := v.calls()
		require.Zero(t, seals)
		require.Zero(t, opens)
	})

	t.Run("failure encoding without encryption is refused", func(t *testing.T) {
		e, err := NewEncryptor(EncryptorConfig{Vault: &fakeVault{}, EncodeFailures: true})
		require.Nil(t, e)
		require.ErrorContains(t, err, "failure encoding requires encryption")
	})
}

func TestEncryptorRoundTrip(t *testing.T) {
	v := &fakeVault{}
	e := requireEncryptor(t, enabledConfig(v))

	req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("one", "two")}
	reply := new(workflowservice.QueryWorkflowResponse)

	err := call(t, e, req, reply, func() error {
		// This is the upstream's view of the request: both payloads sealed, and
		// neither still carrying its plaintext.
		require.Equal(t, 2, sealedCount(req.Input))
		require.NotContains(t, data(req.Input), "one")
		require.NotContains(t, data(req.Input), "two")

		// Hand the sealed payloads straight back so the response has to be
		// opened again on the way in.
		reply.QueryResult = req.Input

		return nil
	})
	require.NoError(t, err)

	// Inbound restored the payloads whole, metadata included, not just the bytes.
	require.Equal(t, []string{"one", "two"}, data(reply.QueryResult))
	require.Zero(t, sealedCount(reply.QueryResult))
	require.Equal(t,
		map[string][]byte{codec.MetadataEncoding: []byte("json/plain")},
		reply.QueryResult.Payloads[0].Metadata,
	)

	// Both seals went to the namespace the context named, not to a default.
	require.Equal(t, []string{testNamespace, testNamespace}, v.namespaces())

	_, opens := v.calls()
	require.Equal(t, 2, opens)
}

func TestEncryptorReverse(t *testing.T) {
	// Reversed is the view from the inbound side: the request comes from a peer
	// that only ever saw sealed payloads, and the response goes back to it.
	v := &fakeVault{}
	e := requireEncryptor(t, EncryptorConfig{Enabled: true, Vault: v, Reverse: true})

	req := &workflowservice.StartWorkflowExecutionRequest{Input: sealed(t, "one", "two")}
	reply := new(workflowservice.QueryWorkflowResponse)

	err := call(t, e, req, reply, func() error {
		// The local cluster's view of the request: opened, metadata and all.
		require.Equal(t, []string{"one", "two"}, data(req.Input))
		require.Zero(t, sealedCount(req.Input))

		reply.QueryResult = payloads("three")

		return nil
	})
	require.NoError(t, err)

	// The peer's view of the response: sealed, under the stamped namespace.
	require.Equal(t, 1, sealedCount(reply.QueryResult))
	require.NotContains(t, data(reply.QueryResult), "three")
	require.Equal(t, []string{testNamespace}, v.namespaces())

	_, opens := v.calls()
	require.Equal(t, 2, opens)
}

func TestEncryptorFailures(t *testing.T) {
	want := appFailure("secret message", "secret stack")

	t.Run("sealed on the way out and restored on the way back", func(t *testing.T) {
		e := requireEncryptor(t, EncryptorConfig{Enabled: true, Vault: &fakeVault{}, EncodeFailures: true})

		req := &workflowservice.RespondActivityTaskFailedRequest{Failure: proto.Clone(want).(*failure.Failure)}
		reply := new(workflowservice.DescribeWorkflowExecutionResponse)

		err := call(t, e, req, reply, func() error {
			requireFailureSealed(t, req.GetFailure())
			reply.PendingActivities = []*workflow.PendingActivityInfo{{LastFailure: req.GetFailure()}}
			return nil
		})
		require.NoError(t, err)

		got := reply.GetPendingActivities()[0].GetLastFailure()
		require.True(t, proto.Equal(want, got), "got %v", got)
	})

	t.Run("reverse restores requests and seals responses", func(t *testing.T) {
		e := requireEncryptor(t, EncryptorConfig{Enabled: true, Vault: &fakeVault{}, EncodeFailures: true, Reverse: true})

		req := &workflowservice.RespondActivityTaskFailedRequest{Failure: sealedFailure(t, want)}
		reply := new(workflowservice.DescribeWorkflowExecutionResponse)

		err := call(t, e, req, reply, func() error {
			require.True(t, proto.Equal(want, req.GetFailure()), "local cluster got %v", req.GetFailure())
			reply.PendingActivities = []*workflow.PendingActivityInfo{{LastFailure: proto.Clone(want).(*failure.Failure)}}
			return nil
		})
		require.NoError(t, err)

		requireFailureSealed(t, reply.GetPendingActivities()[0].GetLastFailure())
	})

	t.Run("flag off sends plaintext but still restores", func(t *testing.T) {
		e := requireEncryptor(t, enabledConfig(&fakeVault{}))

		req := &workflowservice.RespondActivityTaskFailedRequest{Failure: proto.Clone(want).(*failure.Failure)}
		reply := new(workflowservice.DescribeWorkflowExecutionResponse)

		err := call(t, e, req, reply, func() error {
			require.True(t, proto.Equal(want, req.GetFailure()), "sent %v", req.GetFailure())
			reply.PendingActivities = []*workflow.PendingActivityInfo{{LastFailure: sealedFailure(t, want)}}
			return nil
		})
		require.NoError(t, err)

		got := reply.GetPendingActivities()[0].GetLastFailure()
		require.True(t, proto.Equal(want, got), "got %v", got)
	})

	t.Run("SDK-encoded failures round trip unchanged", func(t *testing.T) {
		e := requireEncryptor(t, EncryptorConfig{Enabled: true, Vault: &fakeVault{}, EncodeFailures: true})
		sdk := sdkEncodedFailure()

		req := &workflowservice.RespondActivityTaskFailedRequest{Failure: proto.Clone(sdk).(*failure.Failure)}
		reply := new(workflowservice.DescribeWorkflowExecutionResponse)

		err := call(t, e, req, reply, func() error {
			attrs := req.GetFailure().GetEncodedAttributes()
			require.Equal(t, codec.EncryptionEncoding, string(attrs.GetMetadata()[codec.MetadataEncoding]))
			reply.PendingActivities = []*workflow.PendingActivityInfo{{LastFailure: req.GetFailure()}}
			return nil
		})
		require.NoError(t, err)

		got := reply.GetPendingActivities()[0].GetLastFailure()
		require.True(t, proto.Equal(sdk, got), "got %v", got)
	})

	t.Run("failures in error details are restored", func(t *testing.T) {
		e := requireEncryptor(t, enabledConfig(&fakeVault{}))

		st, err := status.New(codes.FailedPrecondition, "nope").
			WithDetails(&errordetails.QueryFailedFailure{Failure: sealedFailure(t, want)})
		require.NoError(t, err)

		got := call(t, e, &workflowservice.QueryWorkflowRequest{}, new(workflowservice.QueryWorkflowResponse), st.Err)
		require.Equal(t, codes.FailedPrecondition, status.Code(got))

		details := status.Convert(got).Details()
		require.Len(t, details, 1)
		qf, ok := details[0].(*errordetails.QueryFailedFailure)
		require.True(t, ok)
		require.True(t, proto.Equal(want, qf.GetFailure()), "got %v", qf.GetFailure())
	})

	t.Run("reverse seals failures in error details", func(t *testing.T) {
		e := requireEncryptor(t, EncryptorConfig{Enabled: true, Vault: &fakeVault{}, EncodeFailures: true, Reverse: true})

		st, err := status.New(codes.FailedPrecondition, "nope").
			WithDetails(&errordetails.QueryFailedFailure{Failure: proto.Clone(want).(*failure.Failure)})
		require.NoError(t, err)

		got := call(t, e, &workflowservice.QueryWorkflowRequest{}, new(workflowservice.QueryWorkflowResponse), st.Err)
		require.Equal(t, codes.FailedPrecondition, status.Code(got))

		details := status.Convert(got).Details()
		require.Len(t, details, 1)
		qf, ok := details[0].(*errordetails.QueryFailedFailure)
		require.True(t, ok)
		requireFailureSealed(t, qf.GetFailure())
	})
}

func TestEncryptorDoesNotSealTwice(t *testing.T) {
	t.Run("payloads it sealed itself", func(t *testing.T) {
		// Another layer would round-trip through this proxy, but anything opening
		// the payload outside it would peel one layer and find another.
		v := &fakeVault{}
		e := requireEncryptor(t, enabledConfig(v))

		already := sealed(t, "already")
		mixed := &common.Payloads{Payloads: []*common.Payload{
			already.Payloads[0],
			payloads("plain").Payloads[0],
		}}
		want := proto.Clone(already.Payloads[0])

		req := &workflowservice.StartWorkflowExecutionRequest{Input: mixed}
		require.NoError(t, call(t, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
			require.Equal(t, 2, sealedCount(req.Input))
			require.True(t, proto.Equal(want, req.Input.Payloads[0]), "the sealed payload is left exactly as it was")
			require.NotContains(t, data(req.Input), "plain")

			return nil
		}))

		seals, _ := v.calls()
		require.Equal(t, 1, seals, "only the plaintext payload is sealed")
	})

	t.Run("not payloads that only share its encoding name", func(t *testing.T) {
		// Another codec marking its output binary/encrypted, without the key
		// material ours carries, is not ours to vouch for. Sealing it costs an
		// extra layer; trusting the name could put plaintext on the wire.
		v := &fakeVault{}
		e := requireEncryptor(t, enabledConfig(v))

		foreign := &common.Payload{
			Metadata: map[string][]byte{
				codec.MetadataEncoding:        []byte(codec.EncryptionEncoding),
				codec.MetadataEncryptionKeyID: []byte("their-key"),
			},
			Data: []byte("maybe-plaintext"),
		}

		req := &workflowservice.StartWorkflowExecutionRequest{Input: &common.Payloads{Payloads: []*common.Payload{foreign}}}
		require.NoError(t, call(t, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
			require.NotContains(t, data(req.Input), "maybe-plaintext")

			return nil
		}))

		seals, _ := v.calls()
		require.Equal(t, 1, seals)
	})

	t.Run("payloads in an encoding the config names as already sealed", func(t *testing.T) {
		v := &fakeVault{}
		e := requireEncryptor(t, EncryptorConfig{
			Enabled:                true,
			Vault:                  v,
			AlreadySealedEncodings: []string{"binary/customer-encrypted"},
		})

		theirs := &common.Payload{
			Metadata: map[string][]byte{codec.MetadataEncoding: []byte("binary/customer-encrypted")},
			Data:     []byte("their-ciphertext"),
		}
		want := proto.Clone(theirs)

		req := &workflowservice.StartWorkflowExecutionRequest{Input: &common.Payloads{Payloads: []*common.Payload{theirs}}}
		require.NoError(t, call(t, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
			require.True(t, proto.Equal(want, req.Input.Payloads[0]))

			return nil
		}))

		seals, _ := v.calls()
		require.Zero(t, seals)
	})
}

func TestEncryptorNamespaceComesFromTheContext(t *testing.T) {
	// The namespace is resolved per request, not captured when the Encryptor was
	// built, so two requests on differently stamped contexts seal under
	// different keys.
	v := &fakeVault{}
	e := requireEncryptor(t, enabledConfig(v))

	for _, ns := range []string{"tenant-a", "tenant-b"} {
		ctx := context.WithValue(t.Context(), NamespaceKey, ns)
		req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("secret")}

		require.NoError(t, callWith(t, ctx, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
			return nil
		}))
	}

	require.Equal(t, []string{"tenant-a", "tenant-b"}, v.namespaces())
}

func TestEncryptorUnnamedNamespace(t *testing.T) {
	// A request that names no namespace still gets sealed, under whatever key
	// the default policy picks. Encryption config requires a Default precisely
	// so this case has an answer; refusing the request instead would take
	// replication down over a namespace name we were never going to have, and
	// the payload would be no safer for it.
	v := &fakeVault{}
	e := requireEncryptor(t, enabledConfig(v))

	req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("secret")}

	require.NoError(t, callWith(t, t.Context(), e, req, new(workflowservice.QueryWorkflowResponse), func() error {
		require.Equal(t, 1, sealedCount(req.Input), "sealed, just not under a named key")

		return nil
	}))

	require.Equal(t, []string{""}, v.namespaces(), "the vault falls back to its default policy")
}

func TestEncryptorDisabled(t *testing.T) {
	t.Run("no vault leaves everything alone", func(t *testing.T) {
		e := requireEncryptor(t, EncryptorConfig{})

		req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("outgoing")}
		reply := &workflowservice.QueryWorkflowResponse{QueryResult: sealed(t, "incoming")}

		require.NoError(t, call(t, e, req, reply, func() error {
			require.Equal(t, []string{"outgoing"}, data(req.Input))

			return nil
		}))

		// Neither direction is visited, so even a sealed response comes back
		// exactly as the upstream sent it.
		require.Equal(t, 1, sealedCount(reply.QueryResult))
	})

	t.Run("a vault does not seal outbound", func(t *testing.T) {
		v := &fakeVault{}
		e := requireEncryptor(t, EncryptorConfig{Vault: v})

		req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("one", "two")}
		reply := &workflowservice.QueryWorkflowResponse{QueryResult: payloads("three")}

		require.NoError(t, call(t, e, req, reply, func() error {
			require.Equal(t, []string{"one", "two"}, data(req.Input))
			require.Zero(t, sealedCount(req.Input))

			return nil
		}))

		// The inbound side is live here, but the response was never sealed, so it
		// passes through and the vault is never asked to open anything.
		require.Equal(t, []string{"three"}, data(reply.QueryResult))

		seals, opens := v.calls()
		require.Zero(t, seals)
		require.Zero(t, opens)
	})

	t.Run("a vault still opens sealed responses, with no namespace anywhere", func(t *testing.T) {
		// Switching encryption off must not strand data that was sealed while it
		// was on. Note the context carries no namespace: opening needs none,
		// because the message names its own key.
		v := &fakeVault{}
		e := requireEncryptor(t, EncryptorConfig{Vault: v})

		req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("outgoing")}
		reply := &workflowservice.QueryWorkflowResponse{QueryResult: sealed(t, "incoming")}

		require.NoError(t, callWith(t, t.Context(), e, req, reply, func() error {
			require.Zero(t, sealedCount(req.Input), "outbound is still plaintext")

			return nil
		}))

		require.Equal(t, []string{"incoming"}, data(reply.QueryResult))

		seals, opens := v.calls()
		require.Zero(t, seals, "nothing outbound was sealed")
		require.Equal(t, 1, opens)
	})
}

func TestEncryptorLeavesSearchAttributesAlone(t *testing.T) {
	// The server indexes and queries search attributes, so sealing one would
	// make it unusable. Everything else on the same message is still sealed.
	e := requireEncryptor(t, enabledConfig(&fakeVault{}))

	req := &workflowservice.StartWorkflowExecutionRequest{
		Input: payloads("secret"),
		SearchAttributes: &common.SearchAttributes{
			IndexedFields: map[string]*common.Payload{
				"CustomKeywordField": {Data: []byte("searchable")},
			},
		},
	}

	require.NoError(t, call(t, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
		require.Equal(t, 1, sealedCount(req.Input))
		require.Equal(t, []byte("searchable"), req.SearchAttributes.IndexedFields["CustomKeywordField"].Data)

		return nil
	}))
}

func TestEncryptorVaultFailures(t *testing.T) {
	boom := errors.New("kms unreachable")

	t.Run("a failed seal fails the RPC before it is sent", func(t *testing.T) {
		e := requireEncryptor(t, enabledConfig(&fakeVault{sealErr: boom}))

		req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("secret")}

		var invoked bool
		err := call(t, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
			invoked = true

			return nil
		})

		require.ErrorIs(t, err, boom)
		require.False(t, invoked, "plaintext must not reach the upstream when sealing fails")
		require.Equal(t, []string{"secret"}, data(req.Input), "the request is left as it was")
	})

	t.Run("a failed open fails the RPC", func(t *testing.T) {
		e := requireEncryptor(t, enabledConfig(&fakeVault{openErr: boom}))

		req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("secret")}
		reply := new(workflowservice.QueryWorkflowResponse)

		err := call(t, e, req, reply, func() error {
			reply.QueryResult = req.Input

			return nil
		})

		require.ErrorIs(t, err, boom)
	})

	t.Run("an upstream error is returned as-is", func(t *testing.T) {
		e := requireEncryptor(t, enabledConfig(&fakeVault{}))

		req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("secret")}
		err := call(t, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
			return boom
		})

		require.ErrorIs(t, err, boom)
	})
}

func TestVisitPayloads(t *testing.T) {
	nopOpt := func(ctx context.Context, ns string) codec.Option {
		return codec.WithCipher(&cipher{ctx: ctx, ns: ns, v: &fakeVault{}})
	}

	t.Run("no codecs means no options at all", func(t *testing.T) {
		// nil rather than empty options is what lets a disabled Encryptor skip
		// the traversal instead of walking every message to do nothing to it.
		require.Nil(t, visitPayloads(nil, codec.Chain.Encode))
		require.Nil(t, visitPayloads([]codecOpt{}, codec.Chain.Decode))
	})

	t.Run("options are set for concurrent visits that skip search attributes", func(t *testing.T) {
		opts := visitPayloads([]codecOpt{nopOpt}, codec.Chain.Encode)
		require.NotNil(t, opts)
		require.True(t, opts.SkipSearchAttributes)
		require.Equal(t, runtime.NumCPU(), opts.ConcurrencyLimit)
	})

	t.Run("every codec is rebuilt for the visit, in order", func(t *testing.T) {
		var mu sync.Mutex
		var built []string

		record := func(name string) codecOpt {
			return func(ctx context.Context, ns string) codec.Option {
				require.NotNil(t, ctx, "the visit context reaches the codec")

				mu.Lock()
				built = append(built, name+":"+ns)
				mu.Unlock()

				return nopOpt(ctx, ns)
			}
		}

		var gotChain bool
		opts := visitPayloads(
			[]codecOpt{record("a"), record("b")},
			func(_ codec.Chain, ps []*common.Payload) ([]*common.Payload, error) {
				gotChain = true

				return ps, nil
			},
		)

		in := payloads("x").Payloads
		ctx := context.WithValue(t.Context(), NamespaceKey, testNamespace)

		out, err := opts.Visitor(&proxy.VisitPayloadsContext{Context: ctx}, in)
		require.NoError(t, err)
		require.True(t, gotChain)
		require.Equal(t, in, out)
		require.Equal(t, []string{"a:" + testNamespace, "b:" + testNamespace}, built)
	})

	t.Run("an unnamed namespace builds the codecs anyway", func(t *testing.T) {
		// The empty namespace reaches the codec rather than stopping the visit,
		// which is what lets the vault answer with its default policy.
		var got []string
		opts := visitPayloads(
			[]codecOpt{func(ctx context.Context, ns string) codec.Option {
				got = append(got, ns)

				return nopOpt(ctx, ns)
			}},
			func(_ codec.Chain, ps []*common.Payload) ([]*common.Payload, error) {
				return ps, nil
			},
		)

		out, err := opts.Visitor(&proxy.VisitPayloadsContext{Context: t.Context()}, payloads("x").Payloads)
		require.NoError(t, err)
		require.NotNil(t, out)
		require.Equal(t, []string{""}, got)
	})
}

func (f *fakeVault) Seal(_ context.Context, ns string, data []byte, _ ...crypto.SealOption) (*crypto.Message, error) {
	if f.sealErr != nil {
		return nil, f.sealErr
	}

	f.mu.Lock()
	f.sealed = append(f.sealed, ns)
	f.mu.Unlock()

	return &crypto.Message{
		Ciphertext:  scramble(data),
		KeyMaterial: &crypto.DEKMaterial{KEKID: "kek-" + ns, EncryptedDEK: "dek-" + ns},
	}, nil
}

func (f *fakeVault) Open(_ context.Context, m *crypto.Message, _ ...crypto.OpenOption) ([]byte, error) {
	if f.openErr != nil {
		return nil, f.openErr
	}

	f.mu.Lock()
	f.opened = append(f.opened, m.KeyMaterial.KEKID)
	f.mu.Unlock()

	return scramble(m.Ciphertext), nil
}

func (f *fakeVault) calls() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.sealed), len(f.opened)
}

func (f *fakeVault) namespaces() []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.sealed...)
}

func scramble(b []byte) []byte {
	out := make([]byte, len(b))
	for i, c := range b {
		out[i] = c ^ 0xff
	}

	return out
}

// enabledConfig is the shape a fully switched-on Encryptor needs.
func enabledConfig(v Vault) EncryptorConfig {
	return EncryptorConfig{Enabled: true, Vault: v}
}

// payloads builds the Payloads a request carries, one plaintext payload per
// value.
func payloads(vals ...string) *common.Payloads {
	ps := make([]*common.Payload, len(vals))
	for i, v := range vals {
		ps[i] = &common.Payload{
			Metadata: map[string][]byte{codec.MetadataEncoding: []byte("json/plain")},
			Data:     []byte(v),
		}
	}

	return &common.Payloads{Payloads: ps}
}

// sealed builds the Payloads a sealed response carries. It seals through a
// throwaway vault so the one under test records only what the interceptor asks
// of it; scramble is stateless, so any fakeVault opens what another sealed.
func sealed(t *testing.T, vals ...string) *common.Payloads {
	t.Helper()

	chain := codec.NewChain(codec.WithCipher(&cipher{
		ctx: t.Context(),
		ns:  testNamespace,
		v:   &fakeVault{},
	}))

	ps, err := chain.Encode(payloads(vals...).Payloads)
	require.NoError(t, err)

	return &common.Payloads{Payloads: ps}
}

// data pulls the payload bytes back out, so a test can compare against what it
// put in.
func data(ps *common.Payloads) []string {
	out := make([]string, len(ps.GetPayloads()))
	for i, p := range ps.GetPayloads() {
		out[i] = string(p.GetData())
	}

	return out
}

// sealedCount reports how many of ps carry the sealed-payload encoding marker.
func sealedCount(ps *common.Payloads) int {
	var n int
	for _, p := range ps.GetPayloads() {
		if string(p.GetMetadata()[codec.MetadataEncoding]) == codec.EncryptionEncoding {
			n++
		}
	}

	return n
}

// call runs e over req and reply on a context carrying testNamespace, the way
// the server side is expected to leave it.
func call(t *testing.T, e *Encryptor, req, reply any, invoke func() error) error {
	t.Helper()

	return callWith(t, context.WithValue(t.Context(), NamespaceKey, testNamespace), e, req, reply, invoke)
}

// callWith is call for the tests that need to say exactly what is on the
// context, including nothing at all.
func callWith(t *testing.T, ctx context.Context, e *Encryptor, req, reply any, invoke func() error) error {
	t.Helper()

	return e.UnaryClientInterceptor(
		ctx,
		"/temporal.api.workflowservice.v1.WorkflowService/StartWorkflowExecution",
		req, reply, nil,
		func(context.Context, string, any, any, *grpc.ClientConn, ...grpc.CallOption) error {
			return invoke()
		},
	)
}

func requireEncryptor(t *testing.T, cfg EncryptorConfig) *Encryptor {
	t.Helper()

	e, err := NewEncryptor(cfg)
	require.NoError(t, err)
	require.NotNil(t, e)

	return e
}

// testKeyURI is a fixed "testing://" key, so vaults built from it in one test
// open each other's messages.
var testKeyURI = "testing://" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))

// realVault is a crypto.Vault under mode, for the tests that need what
// fakeVault cannot see: the label a seal binds to, and whether an open compares
// it.
func realVault(t *testing.T, mode crypto.BindingMode) *crypto.Vault {
	t.Helper()

	kek, err := crypto.NewKeyFactory().Create(t.Context(), testKeyURI)
	require.NoError(t, err)

	r, err := crypto.NewKEKRegistry(crypto.WithDefaultKey(kek))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	v, err := crypto.NewVault(r,
		crypto.WithDefaultKeyConfig(crypto.KeyConfig{Duration: time.Hour}),
		crypto.WithBindingMode(mode),
	)
	require.NoError(t, err)

	return v
}

// sealedFor seals vals through v bound to label, the way a peer holds them. A
// vault under crypto.BindingOff ignores the label and seals them unbound.
func sealedFor(t *testing.T, v Vault, label string, vals ...string) *common.Payloads {
	t.Helper()

	opt := cipherOpt(v, labelBinder(func(string) string { return label }), nil)
	ps, err := codec.NewChain(opt(t.Context(), testNamespace)).Encode(payloads(vals...).Payloads)
	require.NoError(t, err)

	return &common.Payloads{Payloads: ps}
}

// boundTo reports the label each of ps is bound to, or "<unbound>".
func boundTo(ps *common.Payloads) []string {
	out := make([]string, len(ps.GetPayloads()))
	for i, p := range ps.GetPayloads() {
		ns, ok := p.GetMetadata()[codec.MetadataEncryptionNamespace]
		if !ok {
			out[i] = "<unbound>"
			continue
		}
		out[i] = string(ns)
	}

	return out
}

// recordingBindingObserver remembers every count it is asked to make.
type recordingBindingObserver struct {
	mu                  sync.Mutex
	unbound, mismatched []string
}

func (o *recordingBindingObserver) UnboundOpen(ns string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.unbound = append(o.unbound, ns)
}

func (o *recordingBindingObserver) NamespaceMismatch(ns string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.mismatched = append(o.mismatched, ns)
}

// peerLabel is the label func the tests use: a fixed rename, standing in for
// namespaceTranslation.
func peerLabel(ns string) string { return "peer-" + ns }

func TestEncryptorBindsToTheLabel(t *testing.T) {
	v := realVault(t, crypto.BindingPermissive)
	e := requireEncryptor(t, EncryptorConfig{Enabled: true, Vault: v, Label: peerLabel})

	req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("one")}
	reply := new(workflowservice.QueryWorkflowResponse)

	require.NoError(t, call(t, e, req, reply, func() error {
		require.Equal(t, []string{"peer-" + testNamespace}, boundTo(req.Input))
		reply.QueryResult = req.Input
		return nil
	}))

	require.Equal(t, []string{"one"}, data(reply.QueryResult), "the response opened against the same label")
}

func TestEncryptorWithoutALabelBindsToTheKeyNamespace(t *testing.T) {
	v := realVault(t, crypto.BindingPermissive)
	e := requireEncryptor(t, enabledConfig(v))

	req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("one")}
	require.NoError(t, call(t, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
		require.Equal(t, []string{testNamespace}, boundTo(req.Input))
		return nil
	}))
}

func TestEncryptorBindingOnOpen(t *testing.T) {
	// The response comes back from the peer already sealed, the way it holds
	// payloads, and is opened with the label of the namespace on the context.
	open := func(t *testing.T, mode crypto.BindingMode, held *common.Payloads) (*workflowservice.QueryWorkflowResponse, *recordingBindingObserver, error) {
		obs := &recordingBindingObserver{}
		e := requireEncryptor(t, EncryptorConfig{Enabled: true, Vault: realVault(t, mode), Label: peerLabel, Binding: obs})

		reply := new(workflowservice.QueryWorkflowResponse)
		err := call(t, e, &workflowservice.StartWorkflowExecutionRequest{}, reply, func() error {
			reply.QueryResult = held
			return nil
		})

		return reply, obs, err
	}

	t.Run("strict opens a payload bound to the label", func(t *testing.T) {
		held := sealedFor(t, realVault(t, crypto.BindingPermissive), peerLabel(testNamespace), "mine")

		reply, obs, err := open(t, crypto.BindingStrict, held)
		require.NoError(t, err)
		require.Equal(t, []string{"mine"}, data(reply.QueryResult))
		require.Empty(t, obs.unbound)
		require.Empty(t, obs.mismatched)
	})

	t.Run("strict refuses a mismatch", func(t *testing.T) {
		held := sealedFor(t, realVault(t, crypto.BindingPermissive), "elsewhere", "theirs")

		_, obs, err := open(t, crypto.BindingStrict, held)
		require.ErrorIs(t, err, crypto.ErrNamespaceMismatch)
		require.Equal(t, []string{testNamespace}, obs.mismatched, "counted before the vault refused it")
	})

	t.Run("permissive opens a mismatch and counts it", func(t *testing.T) {
		// The same payload strict refused above: moving back to permissive makes
		// it readable again.
		held := sealedFor(t, realVault(t, crypto.BindingPermissive), "elsewhere", "theirs")

		reply, obs, err := open(t, crypto.BindingPermissive, held)
		require.NoError(t, err)
		require.Equal(t, []string{"theirs"}, data(reply.QueryResult))
		require.Equal(t, []string{testNamespace}, obs.mismatched)
	})

	t.Run("strict refuses an unbound payload", func(t *testing.T) {
		held := sealedFor(t, realVault(t, crypto.BindingOff), "ignored", "legacy")

		_, obs, err := open(t, crypto.BindingStrict, held)
		require.ErrorIs(t, err, crypto.ErrUnbound)
		require.Equal(t, []string{testNamespace}, obs.unbound)
	})

	t.Run("permissive opens an unbound payload and counts it", func(t *testing.T) {
		held := sealedFor(t, realVault(t, crypto.BindingOff), "ignored", "legacy")

		reply, obs, err := open(t, crypto.BindingPermissive, held)
		require.NoError(t, err)
		require.Equal(t, []string{"legacy"}, data(reply.QueryResult))
		require.Equal(t, []string{testNamespace}, obs.unbound)
		require.Empty(t, obs.mismatched)
	})
}
