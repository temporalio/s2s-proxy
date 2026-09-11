package vault

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	// Vault is a [crypto.Vault] that owns the KEKs behind it. Everything a
	// crypto.Vault does is available here; what this adds is Close.
	//
	// Those keys are the only resource a vault holds and nothing else releases
	// them, so a Vault that is never closed holds them for as long as the process
	// runs.
	Vault struct {
		*crypto.Vault

		registry *crypto.KEKRegistry
	}

	// Config is everything [New] needs: the encryption config describing the
	// keys, plus where the resulting vault reports what it does.
	Config struct {
		// Logger each key registration is reported to, with the key URI run
		// through [safeKeyString] so a local key's material stays out of the log.
		// Optional: a nil Logger drops those lines.
		Logger log.Logger
		// Encryption names the keys and how often the DEKs they wrap rotate.
		Encryption config.EncryptionConfig
		// Meter the vault and its keys report to. Optional: a nil Meter becomes
		// [NewCryptoMeter], which reports to the process-wide collectors, so pass
		// one only to report somewhere else.
		Meter CryptoMeter
	}
)

// New builds a [Vault] from cfg. It opens every KEK the config names, registers
// each against the namespace it serves, and applies that namespace's DEK
// rotation schedule; namespaces with no override get the default policy.
// Opening a KEK goes through the KMS driver its scheme names, which is what ctx
// bounds.
//
// cfg.Encryption must have encryption enabled; a config that turned it off gets
// an error rather than a vault it never asked for. The config is validated
// after that, so a bad key URI or an impossible rotation window is reported
// before a single key is opened, and a failure part way through releases
// whatever did open. An error therefore never leaves a key behind.
//
// Close the vault when it is done with.
func New(ctx context.Context, cfg Config) (*Vault, error) {
	// A vault is only meaningful for a config that asked for one. This is also
	// what makes Default safe to read below: validation requires a default policy
	// only when encryption is enabled.
	if !cfg.Encryption.Enabled {
		return nil, errors.New("encryption is disabled: check Enabled before building a vault")
	}

	if err := cfg.Encryption.Validate(); err != nil {
		return nil, fmt.Errorf("invalid encryption config: %w", err)
	}

	if cfg.Logger == nil {
		cfg.Logger = log.NewNoopLogger()
	}

	if cfg.Meter == nil {
		cfg.Meter = NewCryptoMeter()
	}

	opts := make([]crypto.VaultOption, 0, 3+len(cfg.Encryption.Overrides))
	opts = append(opts,
		crypto.WithDefaultKeyConfig(crypto.KeyConfig{
			Duration:    cfg.Encryption.Default.Duration,
			RenewBefore: cfg.Encryption.Default.RenewBefore,
		}),
		crypto.WithCacheSize(cfg.Encryption.CacheSize),
		crypto.WithObserver(cfg.Meter),
	)

	// Each override carries its own DEK lifetime; register it so the override
	// namespace rotates on its own schedule rather than inheriting the default.
	for _, ns := range slices.Sorted(maps.Keys(cfg.Encryption.Overrides)) {
		policy := cfg.Encryption.Overrides[ns]
		opts = append(opts, crypto.WithKeyConfig(ns, crypto.KeyConfig{
			Duration:    policy.Duration,
			RenewBefore: policy.RenewBefore,
		}))
	}

	r, err := createRegistry(ctx, registryConfig{
		ec:  cfg.Encryption,
		kf:  NewKeyFactory(cfg.Meter),
		log: cfg.Logger,
	})
	if err != nil {
		return nil, err
	}

	v, err := crypto.NewVault(r, opts...)
	if err != nil {
		// The registry is ours, and no Vault is going out to carry it, so closing
		// it here is the only thing that will.
		return nil, errors.Join(fmt.Errorf("failed to create crypto vault: %w", err), r.Close())
	}

	return &Vault{Vault: v, registry: r}, nil
}

// Close releases the KEKs the vault holds. It is safe to call more than once:
// every call after the first returns the first one's error.
func (v *Vault) Close() error {
	return v.registry.Close()
}
