package interceptor

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/codec"
	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/proxy"
	"go.temporal.io/api/workflowservice/v1"
	"google.golang.org/grpc"
)

const testNamespace = "some-namespace"

type (
	// nsKey stands in for whatever key the server side will eventually stamp the
	// incoming namespace under. Its only requirement is that it be comparable.
	nsKey struct{}

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
		e, err := NewEncryptor(EncryptorConfig{Enabled: true, NamespaceKey: nsKey{}})
		require.Nil(t, e)
		require.ErrorContains(t, err, "encryption requires a vault")
	})

	t.Run("encryption enabled without a namespace key is refused", func(t *testing.T) {
		e, err := NewEncryptor(EncryptorConfig{Enabled: true, Vault: &fakeVault{}})
		require.Nil(t, e)
		require.ErrorContains(t, err, "encryption requires a namespace context key")
	})

	t.Run("an uncomparable namespace key is refused", func(t *testing.T) {
		// Looking a value up under this key would panic inside a visitor
		// goroutine, so it is refused at construction instead.
		e, err := NewEncryptor(EncryptorConfig{
			Enabled:      true,
			Vault:        &fakeVault{},
			NamespaceKey: []string{"namespace"},
		})
		require.Nil(t, e)
		require.ErrorContains(t, err, "is not comparable")
	})

	t.Run("encryption disabled needs neither", func(t *testing.T) {
		requireEncryptor(t, EncryptorConfig{})
	})

	t.Run("a vault without encryption needs no namespace key", func(t *testing.T) {
		// Only sealing needs a namespace, and this shape never seals.
		v := &fakeVault{}
		requireEncryptor(t, EncryptorConfig{Vault: v})

		seals, opens := v.calls()
		require.Zero(t, seals)
		require.Zero(t, opens)
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

func TestEncryptorNamespaceComesFromTheContext(t *testing.T) {
	// The namespace is resolved per request, not captured when the Encryptor was
	// built, so two requests on differently stamped contexts seal under
	// different keys.
	v := &fakeVault{}
	e := requireEncryptor(t, enabledConfig(v))

	for _, ns := range []string{"tenant-a", "tenant-b"} {
		ctx := context.WithValue(t.Context(), nsKey{}, ns)
		req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("secret")}

		require.NoError(t, callWith(t, ctx, e, req, new(workflowservice.QueryWorkflowResponse), func() error {
			return nil
		}))
	}

	require.Equal(t, []string{"tenant-a", "tenant-b"}, v.namespaces())
}

func TestEncryptorMissingNamespace(t *testing.T) {
	// Nothing stamps the namespace yet, so this is the state the proxy is in
	// until that lands. Sealing under whatever the default policy names would be
	// the quiet failure; failing the RPC is the loud one.
	v := &fakeVault{}
	e := requireEncryptor(t, enabledConfig(v))

	req := &workflowservice.StartWorkflowExecutionRequest{Input: payloads("secret")}

	var invoked bool
	err := callWith(t, t.Context(), e, req, new(workflowservice.QueryWorkflowResponse), func() error {
		invoked = true

		return nil
	})

	require.ErrorContains(t, err, "no namespace on the request context")
	require.False(t, invoked, "plaintext must not reach the upstream")
	require.Equal(t, []string{"secret"}, data(req.Input), "the request is left as it was")

	seals, _ := v.calls()
	require.Zero(t, seals)
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
		// was on. Note the config has no NamespaceKey and the context carries no
		// namespace: opening needs neither, because the message names its own key.
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

func TestNamespaceFrom(t *testing.T) {
	resolve := namespaceFrom(nsKey{})

	t.Run("returns the namespace the context carries", func(t *testing.T) {
		ns, err := resolve(context.WithValue(t.Context(), nsKey{}, testNamespace))
		require.NoError(t, err)
		require.Equal(t, testNamespace, ns)
	})

	for name, ctx := range map[string]func(*testing.T) context.Context{
		"nothing under the key": func(t *testing.T) context.Context {
			return t.Context()
		},
		"a value of the wrong type": func(t *testing.T) context.Context {
			return context.WithValue(t.Context(), nsKey{}, 42)
		},
		"an empty namespace": func(t *testing.T) context.Context {
			return context.WithValue(t.Context(), nsKey{}, "")
		},
		"a value under a different key": func(t *testing.T) context.Context {
			type otherKey struct{}

			return context.WithValue(t.Context(), otherKey{}, testNamespace)
		},
	} {
		t.Run(name+" is an error, not a default", func(t *testing.T) {
			ns, err := resolve(ctx(t))
			require.Empty(t, ns)
			require.ErrorContains(t, err, "no namespace on the request context")
		})
	}
}

func TestNoNamespace(t *testing.T) {
	// The opening direction resolves to nothing and never fails, because a
	// sealed message names the key that wrapped it.
	ns, err := noNamespace(t.Context())
	require.NoError(t, err)
	require.Empty(t, ns)
}

func TestVisitPayloads(t *testing.T) {
	nopOpt := func(ctx context.Context, ns string) codec.Option {
		return codec.WithCipher(&cipher{ctx: ctx, ns: ns, v: &fakeVault{}})
	}

	t.Run("no codecs means no options at all", func(t *testing.T) {
		// nil rather than empty options is what lets a disabled Encryptor skip
		// the traversal instead of walking every message to do nothing to it.
		require.Nil(t, visitPayloads(nil, codec.Chain.Encode, noNamespace))
		require.Nil(t, visitPayloads([]codecOpt{}, codec.Chain.Decode, noNamespace))
	})

	t.Run("options are set for concurrent visits that skip search attributes", func(t *testing.T) {
		opts := visitPayloads([]codecOpt{nopOpt}, codec.Chain.Encode, noNamespace)
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
			namespaceFrom(nsKey{}),
		)

		in := payloads("x").Payloads
		ctx := context.WithValue(t.Context(), nsKey{}, testNamespace)

		out, err := opts.Visitor(&proxy.VisitPayloadsContext{Context: ctx}, in)
		require.NoError(t, err)
		require.True(t, gotChain)
		require.Equal(t, in, out)
		require.Equal(t, []string{"a:" + testNamespace, "b:" + testNamespace}, built)
	})

	t.Run("an unresolved namespace fails the visit before any codec is built", func(t *testing.T) {
		var built bool
		opts := visitPayloads(
			[]codecOpt{func(ctx context.Context, ns string) codec.Option {
				built = true

				return nopOpt(ctx, ns)
			}},
			codec.Chain.Encode,
			namespaceFrom(nsKey{}),
		)

		out, err := opts.Visitor(&proxy.VisitPayloadsContext{Context: t.Context()}, payloads("x").Payloads)
		require.ErrorContains(t, err, "no namespace on the request context")
		require.Nil(t, out)
		require.False(t, built)
	})
}

func (f *fakeVault) Seal(_ context.Context, ns string, data []byte) (*crypto.Message, error) {
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

func (f *fakeVault) Open(_ context.Context, m *crypto.Message) ([]byte, error) {
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
	return EncryptorConfig{Enabled: true, Vault: v, NamespaceKey: nsKey{}}
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

	return callWith(t, context.WithValue(t.Context(), nsKey{}, testNamespace), e, req, reply, invoke)
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
