package vault

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	// registryConfig is what building a [crypto.KEKRegistry] takes: the config
	// naming the keys, plus the logger and the factory to open them with.
	registryConfig struct {
		log log.Logger
		ec  config.EncryptionConfig
		kf  keyCreator
	}

	// keyCreator opens the KEK a URI addresses. [KeyFactory] is the real one; the
	// indirection is here so a test can watch which keys are opened and whether
	// they are closed again.
	keyCreator interface {
		Create(ctx context.Context, uri string) (crypto.KEK, error)
	}

	// registryBuilder is one [createRegistry] call in progress. It holds onto
	// every key opened along the way, because a failure half way through has to
	// release all of them and not just the ones the namespace that failed had
	// opened.
	registryBuilder struct {
		registryConfig

		opened []crypto.KEK
	}
)

// createRegistry opens the keys opts names and indexes them by the namespace
// each one serves. The caller owns the registry that comes back and must Close
// it; an error leaves nothing open.
func createRegistry(ctx context.Context, opts registryConfig) (*crypto.KEKRegistry, error) {
	b := &registryBuilder{registryConfig: opts}

	kopts, err := b.toOptions(ctx)
	if err != nil {
		b.closeOpened()
		return nil, err
	}

	registry, err := crypto.NewKEKRegistry(kopts...)
	if err != nil {
		b.closeOpened()
		return nil, fmt.Errorf("failed to create KEK registry: %w", err)
	}

	return registry, nil
}

// closeOpened releases every key opened so far, for the paths where no registry
// is returned to close them later. A close error is dropped: the error on its
// way out is why we are here, and it is the one worth reporting.
func (b *registryBuilder) closeOpened() {
	for _, k := range b.opened {
		_ = k.Close()
	}
}

func (b *registryBuilder) toOptions(ctx context.Context) ([]crypto.KEKRegistryOption, error) {
	kopts, err := b.nsOptions(ctx, "default", b.ec.Default)
	if err != nil {
		return nil, err
	}

	for _, ns := range slices.Sorted(maps.Keys(b.ec.Overrides)) {
		pol := b.ec.Overrides[ns]
		res, err := b.nsOptions(ctx, ns, &pol)
		if err != nil {
			return nil, err
		}

		kopts = append(kopts, res...)
	}

	return kopts, nil
}

func (b *registryBuilder) nsOptions(
	ctx context.Context,
	ns string,
	p *config.KeyPolicy,
) ([]crypto.KEKRegistryOption, error) {
	opts := []crypto.KEKRegistryOption{}
	keys, err := b.createKEKs(ctx, ns, p)
	if err != nil {
		return nil, err
	}

	// NB: KeyConfig.URI is required and therefore this will never be out of bounds.
	if p == b.ec.Default {
		opts = append(opts, crypto.WithDefaultKey(keys[0]))
	} else {
		opts = append(opts, crypto.WithKeyForNamespace(ns, keys[0]))
	}

	for i := 1; i < len(keys); i++ {
		opts = append(opts, crypto.WithDecryptOnlyKey(keys[i]))
	}

	return opts, nil
}

// createKEKs opens the keys p names, the policy's own first and its decrypt-only
// keys after it, in the order the caller expects to find them.
func (b *registryBuilder) createKEKs(
	ctx context.Context,
	ns string,
	p *config.KeyPolicy,
) ([]crypto.KEK, error) {
	logger := log.With(b.log, tag.String("namespace", ns))
	keys := make([]crypto.KEK, 0, len(p.DecryptURIs)+1)

	mkKey := func(uri string) error {
		logger.Info("Registering crypto key", tag.String("uri", safeKeyString(uri)))
		k, err := b.kf.Create(ctx, uri)
		if err != nil {
			return err
		}

		// Two slices, two jobs: keys is this namespace's, in order, and b.opened
		// owns every key the build has opened so it can be undone.
		keys = append(keys, k)
		b.opened = append(b.opened, k)

		return nil
	}

	if err := mkKey(p.URI); err != nil {
		return nil, err
	}

	for _, uri := range p.DecryptURIs {
		if err := mkKey(uri); err != nil {
			return nil, err
		}
	}

	return keys, nil
}
