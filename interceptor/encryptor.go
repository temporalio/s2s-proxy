package interceptor

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"slices"

	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/proxy"
	"go.temporal.io/proxy/pkg/codec"
	"go.temporal.io/proxy/pkg/crypto"
	"google.golang.org/grpc"
)

type (
	// Encryptor is the gRPC client interceptor that seals payloads on their way
	// out to an upstream and opens them on the way back. It embeds the
	// interceptor func, so it goes wherever a [grpc.UnaryClientInterceptor] is
	// wanted:
	//
	//	e, err := interceptor.NewEncryptor(interceptor.EncryptorConfig{
	//	    Enabled: cfg.Encryption.Enabled,
	//	    Vault:   v,
	//	})
	//	if err != nil {
	//	    return err
	//	}
	//
	//	conn, err := grpc.NewClient(target, grpc.WithUnaryInterceptor(e.UnaryClientInterceptor))
	//
	// An Encryptor is a usable interceptor whatever its config says, so callers
	// install it unconditionally rather than branching. What the config decides
	// is how much of it does anything; see [NewEncryptor].
	Encryptor struct {
		grpc.UnaryClientInterceptor
	}

	// EncryptorConfig is everything [NewEncryptor] needs: whether to encrypt at
	// all and the keys to do it with. The two are separate so encryption can be
	// switched off without giving up the ability to read what was written while
	// it was on.
	EncryptorConfig struct {
		// Enabled turns outbound sealing on. Requires a Vault.
		Enabled bool
		// Vault seals and opens payload data. Required when Enabled, and not
		// pointless without it: a vault on its own still opens sealed responses.
		Vault Vault
		// Reverse swaps the directions: requests are opened and responses are
		// sealed. It is for the client that carries a peer's calls in to the local
		// cluster, where what arrives was sealed and what leaves has to be.
		Reverse bool
		// AlreadySealedEncodings are payload encodings another codec, typically
		// the customer's own, has already encrypted. Payloads in one are never
		// sealed again; they are not opened either, since they are not ours. Our
		// own sealed payloads are always skipped and need not be listed. Only list
		// encodings that encrypt: a compression codec's would reach the peer
		// readable.
		AlreadySealedEncodings []string
		// EncodeFailures moves the message and stack trace of failures being
		// sealed into their encoded attributes, so they are sealed with every
		// other payload. Requires Enabled. Failures encoded this way are restored
		// on the opening side whenever there is a Vault, whatever this says, so
		// turning it off does not leave the placeholder behind.
		EncodeFailures bool
		// Label maps the local namespace on the context to the label sealed
		// payloads are bound to: the namespace's name on the peer. Opening then
		// expects that label, which the vault only enforces under a strict
		// binding. Nil binds by the key namespace, the vault's default, and
		// compares nothing on open.
		Label func(localNs string) string
		// Binding counts the payloads a strict binding would refuse. Optional.
		Binding BindingObserver
	}

	// BindingObserver hears about sealed payloads a strict namespace binding
	// would refuse, whatever the vault then does with them: those with no
	// binding, and those bound to a namespace other than the expected label.
	// Both take the local namespace. Satisfied by vault.BindingMeter.
	BindingObserver interface {
		UnboundOpen(ns string)
		NamespaceMismatch(ns string)
	}

	// Vault is the subset of a key-management backend this interceptor depends
	// on, satisfied by [crypto.Vault] and by anything that embeds one.
	//
	// Seal takes the namespace whose KEK should wrap the data. Open takes no
	// namespace because a sealed [crypto.Message] carries the ID of the key that
	// wrapped its DEK, which is all the vault needs to find it again.
	Vault interface {
		Seal(context.Context, string, []byte, ...crypto.SealOption) (*crypto.Message, error)
		Open(context.Context, *crypto.Message, ...crypto.OpenOption) ([]byte, error)
	}

	// cipher pins a [Vault] to one namespace, its label, and one request's
	// context so it can satisfy [codec.Cipher], whose Encrypt and Decrypt take
	// none of them. Holding a context in a struct is normally worth avoiding;
	// here it is the only way through that interface, and a cipher never
	// outlives the visit that built it.
	//
	// check is false when there is no label func, so the vault's own default
	// label (ns) applies and opening compares nothing.
	cipher struct {
		ctx   context.Context
		ns    string
		label string
		check bool
		v     Vault
		obs   BindingObserver
	}

	// codecOpt defers a [codec.Option] until the namespace and context are
	// known, which is not until a payload is in front of us.
	codecOpt func(ctx context.Context, ns string) codec.Option
)

// NewEncryptor returns the [Encryptor] cfg describes. The two directions are
// configured separately, so cfg has three meaningful shapes:
//
//   - A vault with Enabled seals the payloads of every outbound request and
//     opens the payloads of every inbound response.
//   - A vault without Enabled only opens. Switching encryption off does not
//     make data sealed while it was on unreadable, so decryption stays on as
//     encryption stops. Payloads that were never sealed pass through the
//     inbound side untouched, so mixed traffic is fine.
//   - No vault and not enabled does nothing, and costs nothing: neither
//     direction is visited at all.
//
// Reverse swaps which direction is which without changing any of the above:
// requests are opened whenever there is a vault, and responses are sealed only
// when Enabled.
//
// With EncodeFailures, the message and stack trace of every failure being sealed
// move into its encoded attributes first, so they are sealed too. Failures
// encoded that way are put back exactly whenever there is a vault.
//
// Inbound covers the payloads carried in the details of a gRPC error as well as
// the ones in the response body. Search attributes are deliberately skipped in
// both directions: the server indexes and queries them, so a sealed one would
// be useless to it.
//
// Enabled with no vault is refused. It reads as "encrypt" but would put
// plaintext on the wire, and a config that is wrong about encryption should not
// start.
func NewEncryptor(cfg EncryptorConfig) (*Encryptor, error) {
	if cfg.Enabled && cfg.Vault == nil {
		return nil, errors.New("proxy: encryption requires a vault")
	}

	// Without sealing, moving the text into a payload hides nothing; it would only
	// cost anyone reading the failure without an SDK its message.
	if cfg.EncodeFailures && !cfg.Enabled {
		return nil, errors.New("proxy: failure encoding requires encryption")
	}

	var seal, open []codecOpt
	if cfg.Vault != nil {
		enc := cipherOpt(cfg.Vault, cfg.Label, cfg.Binding)

		open = append(open, enc)
		if cfg.Enabled {
			seal = append(seal, enc)
		}
	}

	sealFn := sealUnlessSealed(alreadySealed(cfg.AlreadySealedEncodings))
	opts := proxy.PayloadVisitorInterceptorOptions{
		Inbound:  visitPayloads(open, codec.Chain.Decode),
		Outbound: visitPayloads(seal, sealFn),
	}
	if cfg.Reverse {
		opts = proxy.PayloadVisitorInterceptorOptions{
			Inbound:  visitPayloads(seal, sealFn),
			Outbound: visitPayloads(open, codec.Chain.Decode),
		}
	}

	unary, err := proxy.NewPayloadVisitorInterceptor(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create encryption interceptor: %w", err)
	}

	// Restoring needs only a vault, like opening. With no vault there is no
	// failure work in either direction, since encoding requires Enabled.
	if cfg.Vault == nil {
		return &Encryptor{UnaryClientInterceptor: unary}, nil
	}

	var encode *proxy.VisitFailuresOptions
	if cfg.EncodeFailures {
		encode = visitFailureOpts(encodeFailure)
	}
	restore := visitFailureOpts(restoreFailure)

	fopts := proxy.FailureVisitorInterceptorOptions{Inbound: restore, Outbound: encode}
	if cfg.Reverse {
		fopts = proxy.FailureVisitorInterceptorOptions{Inbound: encode, Outbound: restore}
	}

	failures, err := proxy.NewFailureVisitorInterceptor(fopts)
	if err != nil {
		return nil, fmt.Errorf("failed to create failure encryption interceptor: %w", err)
	}

	// Failures are encoded before their attributes are sealed, and restored after
	// they are opened. Whichever interceptor is outer sees the request first and
	// the response last, so the failure one goes outside when requests are sealed
	// and inside when they are opened.
	if cfg.Reverse {
		return &Encryptor{UnaryClientInterceptor: chainUnary(unary, failures)}, nil
	}

	return &Encryptor{UnaryClientInterceptor: chainUnary(failures, unary)}, nil
}

// Encrypt seals data under the namespace this cipher was built for, bound to
// its label when it has one.
func (c *cipher) Encrypt(data []byte) (*crypto.Message, error) {
	if c.check {
		return c.v.Seal(c.ctx, c.ns, data, crypto.WithLabel(c.label))
	}

	return c.v.Seal(c.ctx, c.ns, data)
}

// Decrypt opens m. The cipher's namespace does not pick the key: m names the
// key that wrapped its DEK, so the vault finds it from the message alone. That
// is what lets a payload sealed under an older key, or under a namespace's key
// before its policy changed, still open. With a label, m must be bound to it,
// which the vault enforces under a strict binding only.
//
// Unbound and mismatched messages are counted first, whatever the vault then
// does, so running permissive shows what strict would refuse.
func (c *cipher) Decrypt(m *crypto.Message) ([]byte, error) {
	if c.obs != nil {
		switch {
		case !m.Bound:
			c.obs.UnboundOpen(c.ns)
		case c.check && m.Namespace != c.label:
			c.obs.NamespaceMismatch(c.ns)
		}
	}

	if c.check {
		return c.v.Open(c.ctx, m, crypto.ExpectNamespace(c.label))
	}

	return c.v.Open(c.ctx, m)
}

// cipherOpt returns the codecOpt that seals and opens through v, bound to
// label(ns) when label is set, and reporting what a strict binding would
// refuse to obs when that is set.
func cipherOpt(v Vault, label func(string) string, obs BindingObserver) codecOpt {
	return func(ctx context.Context, ns string) codec.Option {
		c := &cipher{ctx: ctx, ns: ns, v: v, obs: obs}
		if label != nil {
			c.label, c.check = label(ns), true
		}

		return codec.WithCipher(c)
	}
}

// sealUnlessSealed returns a [codec.Chain.Encode] that leaves alone every
// payload [sealed] reports as already sealed, and seals the rest in place.
// Sealing one again would round-trip through this proxy, but anything opening it
// outside the proxy would peel one layer and find another underneath.
func sealUnlessSealed(sealed func(*common.Payload) bool) func(codec.Chain, []*common.Payload) ([]*common.Payload, error) {
	return func(c codec.Chain, payloads []*common.Payload) ([]*common.Payload, error) {
		var at []int
		var todo []*common.Payload
		for i, p := range payloads {
			if !sealed(p) {
				at = append(at, i)
				todo = append(todo, p)
			}
		}

		if len(todo) == 0 {
			return payloads, nil
		}

		done, err := c.Encode(todo)
		if err != nil {
			return nil, err
		}

		out := slices.Clone(payloads)
		for j, i := range at {
			out[i] = done[j]
		}

		return out, nil
	}
}

// alreadySealed reports a payload as already sealed when it is one of ours or
// its encoding is one of encodings.
func alreadySealed(encodings []string) func(*common.Payload) bool {
	return func(p *common.Payload) bool {
		return isSealed(p) || slices.Contains(encodings, string(p.GetMetadata()[codec.MetadataEncoding]))
	}
}

// isSealed reports whether p is one of our sealed payloads. The encoding marker
// alone is not enough, since another codec may use the same name; it takes the
// key material too, the same claim of ownership [codec.Encryptor.Decode] checks
// before it will open one.
func isSealed(p *common.Payload) bool {
	md := p.GetMetadata()
	return string(md[codec.MetadataEncoding]) == codec.EncryptionEncoding &&
		len(md[codec.MetadataEncryptionKeyID]) > 0 &&
		len(md[codec.MetadataEncryptionDEK]) > 0
}

// visitPayloads builds the visit options that run every codec opts enables over
// a message's payloads, through fn: [codec.Chain.Encode] to seal, or
// [codec.Chain.Decode] to open.
//
// The codecs are built for whatever namespace [StampNamespace] left on the
// context, and for the empty string when it left none. Empty is a valid answer
// rather than a failure: the vault falls back to its default key policy, which
// is what that policy is for. Refusing to seal would take down traffic the
// proxy is meant to carry, and the payload would be no safer for it. Opening
// ignores the namespace entirely, since a sealed message names its own key.
//
// The chain is rebuilt on each visit because every [codecOpt] closes over that
// visit's namespace and context. Visits run up to [runtime.NumCPU] at a time,
// so whatever the opts reach, the [Vault] above all, has to be safe for
// concurrent use.
func visitPayloads(
	opts []codecOpt,
	fn func(codec.Chain, []*common.Payload) ([]*common.Payload, error),
) *proxy.VisitPayloadsOptions {
	if len(opts) == 0 {
		return nil
	}

	return &proxy.VisitPayloadsOptions{
		ConcurrencyLimit:     runtime.NumCPU(),
		SkipSearchAttributes: true,
		Visitor: func(ctx *proxy.VisitPayloadsContext, payloads []*common.Payload) ([]*common.Payload, error) {
			namespace, _ := ctx.Value(NamespaceKey).(string)
			chain := make([]codec.Option, len(opts))
			for i, opt := range opts {
				chain[i] = opt(ctx, namespace)
			}

			return fn(codec.NewChain(chain...), payloads)
		},
	}
}
