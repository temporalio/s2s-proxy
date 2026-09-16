package interceptor

import (
	"context"
	"errors"
	"fmt"
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
// Enabled with no vault is refused. It reads as "encrypt" but would put
// plaintext on the wire, and a config that is wrong about encryption should not
// start.
func NewEncryptor(cfg EncryptorConfig) (*Encryptor, error) {
	if cfg.Enabled && cfg.Vault == nil {
		return nil, errors.New("proxy: encryption requires a vault")
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
		Inbound:  visitPayloads(in, codec.Chain.Decode),
		Outbound: visitPayloads(out, codec.Chain.Encode),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create encryption interceptor: %w", err)
	}

	return &Encryptor{UnaryClientInterceptor: unary}, nil
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
