package interceptor

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"runtime"

	"github.com/temporalio/temporal-proxy/pkg/codec"
	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/proxy"
	"google.golang.org/grpc"
)

type (
	// Encryptor is the gRPC client interceptor that seals payloads on their way
	// out to an upstream and opens them on the way back. It embeds the
	// interceptor func, so it goes wherever a [grpc.UnaryClientInterceptor] is
	// wanted:
	//
	//	e, err := interceptor.NewEncryptor(interceptor.EncryptorConfig{
	//	    Enabled:      cfg.Encryption.Enabled,
	//	    Vault:        v,
	//	    NamespaceKey: namespaceKey,
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
	// all, the keys to do it with, and where to find the namespace those keys are
	// chosen by. Enabled and Vault are separate so encryption can be switched off
	// without giving up the ability to read what was written while it was on.
	EncryptorConfig struct {
		// Enabled turns outbound sealing on. Requires a Vault and a NamespaceKey.
		Enabled bool
		// Vault seals and opens payload data. Required when Enabled, and not
		// pointless without it: a vault on its own still opens sealed responses.
		Vault Vault
		// NamespaceKey is the context key the incoming request's namespace is
		// stored under, as given to [context.WithValue]. It must be comparable,
		// for the same reason context.WithValue requires it, and the value it
		// finds must be a string.
		//
		// Required when Enabled and unused otherwise, since only sealing needs a
		// namespace. There is no default: the namespace picks the KEK, so a
		// guess means encrypting a tenant's data under a key that is not theirs.
		NamespaceKey any
	}

	// Vault is the subset of a key-management backend this interceptor depends
	// on, satisfied by [crypto.Vault] and by anything that embeds one.
	//
	// Seal takes the namespace whose KEK should wrap the data. Open takes no
	// namespace because a sealed [crypto.Message] carries the ID of the key that
	// wrapped its DEK, which is all the vault needs to find it again.
	Vault interface {
		Seal(context.Context, string, []byte) (*crypto.Message, error)
		Open(context.Context, *crypto.Message) ([]byte, error)
	}

	// cipher pins a [Vault] to one namespace and one request's context so it can
	// satisfy [codec.Cipher], whose Encrypt and Decrypt take neither. Holding a
	// context in a struct is normally worth avoiding; here it is the only way
	// through that interface, and a cipher never outlives the visit that built
	// it.
	cipher struct {
		ctx context.Context
		ns  string
		v   Vault
	}

	// codecOpt defers a [codec.Option] until the namespace and context are
	// known, which is not until a payload is in front of us.
	codecOpt func(ctx context.Context, ns string) codec.Option

	// namespaceResolver reports the namespace a visit's codecs should be built
	// for, or why it cannot be determined. The two directions resolve
	// differently: see [namespaceFrom] and [noNamespace].
	namespaceResolver func(context.Context) (string, error)
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
// Inbound covers the payloads carried in the details of a gRPC error as well as
// the ones in the response body. Search attributes are deliberately skipped in
// both directions: the server indexes and queries them, so a sealed one would
// be useless to it.
//
// Enabled without a vault, or without a usable NamespaceKey, is refused. Both
// read as "encrypt" while leaving the interceptor unable to encrypt correctly,
// and a config that is wrong about encryption should not start.
func NewEncryptor(cfg EncryptorConfig) (*Encryptor, error) {
	if cfg.Enabled {
		if cfg.Vault == nil {
			return nil, errors.New("proxy: encryption requires a vault")
		}

		if cfg.NamespaceKey == nil {
			return nil, errors.New("proxy: encryption requires a namespace context key")
		}

		// Looking a value up under an uncomparable key panics, and the lookup
		// happens inside a visitor goroutine where a panic takes the process with
		// it. Refuse the key now instead, the way context.WithValue would.
		if !reflect.TypeOf(cfg.NamespaceKey).Comparable() {
			return nil, fmt.Errorf("proxy: namespace context key of type %T is not comparable", cfg.NamespaceKey)
		}
	}

	var out, in []codecOpt
	if cfg.Vault != nil {
		enc := func(ctx context.Context, ns string) codec.Option {
			return codec.WithCipher(&cipher{ctx: ctx, ns: ns, v: cfg.Vault})
		}

		in = append(in, enc)
		if cfg.Enabled {
			out = append(out, enc)
		}
	}

	unary, err := proxy.NewPayloadVisitorInterceptor(proxy.PayloadVisitorInterceptorOptions{
		Inbound:  visitPayloads(in, codec.Chain.Decode, noNamespace),
		Outbound: visitPayloads(out, codec.Chain.Encode, namespaceFrom(cfg.NamespaceKey)),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create encryption interceptor: %w", err)
	}

	return &Encryptor{UnaryClientInterceptor: unary}, nil
}

// namespaceFrom returns the resolver for the sealing direction, reading the
// namespace the server side left on the request context under key.
//
// A context with nothing usable there is an error rather than a fallback. The
// namespace chooses the KEK, so carrying on under "" would seal a tenant's data
// under whichever key the default policy names, which is the quiet version of
// the failure this is meant to prevent. Failing the RPC is the loud one.
func namespaceFrom(key any) namespaceResolver {
	return func(ctx context.Context) (string, error) {
		ns, _ := ctx.Value(key).(string)
		if ns == "" {
			return "", fmt.Errorf("proxy: no namespace on the request context under key %v", key)
		}

		return ns, nil
	}
}

// noNamespace is the resolver for the opening direction, which needs none: a
// sealed message names the key that wrapped its DEK, so [cipher.Decrypt] never
// reads the namespace its cipher was built with. A codec added to that side
// which does care about the namespace would need its own resolver.
func noNamespace(context.Context) (string, error) {
	return "", nil
}

// Encrypt seals data under the namespace this cipher was built for.
func (c *cipher) Encrypt(data []byte) (*crypto.Message, error) {
	return c.v.Seal(c.ctx, c.ns, data)
}

// Decrypt opens m. The cipher's namespace plays no part: m names the key that
// wrapped its DEK, so the vault finds it from the message alone. That is what
// lets a payload sealed under an older key, or under a namespace's key before
// its policy changed, still open.
func (c *cipher) Decrypt(m *crypto.Message) ([]byte, error) {
	return c.v.Open(c.ctx, m)
}

// visitPayloads builds the visit options that run every codec opts enables over
// a message's payloads, through fn: [codec.Chain.Encode] to seal, or
// [codec.Chain.Decode] to open. ns supplies the namespace those codecs are
// built for, and a namespace it cannot resolve fails the request.
//
// Empty opts returns nil, which is how a disabled [Encryptor] ends up with no
// Inbound or Outbound options at all. That matters for more than tidiness: a
// nil side is skipped outright, so a pass-through interceptor never walks a
// message tree to do nothing to it.
//
// The chain is rebuilt on each visit because every [codecOpt] closes over that
// visit's namespace and context. Visits run up to [runtime.NumCPU] at a time,
// so whatever the opts reach, the [Vault] above all, has to be safe for
// concurrent use.
func visitPayloads(
	opts []codecOpt,
	fn func(codec.Chain, []*common.Payload) ([]*common.Payload, error),
	ns namespaceResolver,
) *proxy.VisitPayloadsOptions {
	if len(opts) == 0 {
		return nil
	}

	return &proxy.VisitPayloadsOptions{
		ConcurrencyLimit:     runtime.NumCPU(),
		SkipSearchAttributes: true,
		Visitor: func(ctx *proxy.VisitPayloadsContext, payloads []*common.Payload) ([]*common.Payload, error) {
			namespace, err := ns(ctx)
			if err != nil {
				return nil, err
			}

			chain := make([]codec.Option, len(opts))
			for i, opt := range opts {
				chain[i] = opt(ctx, namespace)
			}

			return fn(codec.NewChain(chain...), payloads)
		},
	}
}
