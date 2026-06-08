package crypto

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"slices"
	"sync"
)

type (
	// KEK defines an interface for Key Encryption Keys.
	// These keys are used to encrypt/decrypt DEKs and are customer-managed (e.g. via AWS/GCP KMS).
	KEK interface {
		io.Closer

		ID() string // A unique ID for this KEK, e.g. KMS ARN
		Encrypt(context.Context, []byte) ([]byte, error)
		Decrypt(context.Context, []byte) ([]byte, error)
	}

	// KEKEncryptor wraps a current [KEK] used to encrypt new DEKs and a set of retired
	// KEKs retained for decryption only. It looks up KEKs by ID when opening
	// [DEKMaterial] so DEKs encrypted with rotated-out keys remain readable.
	KEKEncryptor struct {
		currentKey  KEK
		retiredKeys []KEK
		keys        map[string]KEK // NB: fast lookup by id

		closeOnce sync.Once
		closeErr  error
	}

	// KEKEncryptorOption configures a [KEKEncryptor] during construction.
	KEKEncryptorOption func(*KEKEncryptor)

	// nilKEK defines a [KEK] that does nothing.
	nilKEK struct{}
)

// NewKEKEncryptor constructs a [KEKEncryptor] using the supplied opts.
func NewKEKEncryptor(opts ...KEKEncryptorOption) *KEKEncryptor {
	r := &KEKEncryptor{
		currentKey: new(nilKEK),
		keys:       make(map[string]KEK),
	}
	for _, opt := range opts {
		opt(r)
	}

	r.keys[r.currentKey.ID()] = r.currentKey
	for _, k := range r.retiredKeys {
		r.keys[k.ID()] = k
	}

	return r
}

// WithKey sets the [KEK] used to encrypt new DEKs. Defaults to a no-op KEK when unset.
func WithKey(k KEK) KEKEncryptorOption {
	return func(r *KEKEncryptor) {
		if k != nil { // nil means fallback to nilKEK.
			r.currentKey = k
		}
	}
}

// WithRetiredKey registers k for decryption only. It is added to the key-ID index so that DEKs
// encrypted with k can still be opened, but k is never selected for new DEK encryption.
func WithRetiredKey(k KEK) KEKEncryptorOption {
	return func(r *KEKEncryptor) {
		if !slices.Contains(r.retiredKeys, k) {
			r.retiredKeys = append(r.retiredKeys, k)
		}
	}
}

// Encrypt encrypts the given DEK using the current KEK. It returns DEKMaterial
// containing the KEK ID and the base64-encoded ciphertext.
func (p *KEKEncryptor) Encrypt(ctx context.Context, dek *DEK) (*DEKMaterial, error) {
	k := p.currentKey
	ct, err := k.Encrypt(ctx, dek.key)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt message: %w", err)
	}

	return &DEKMaterial{
		KEKID:        k.ID(),
		EncryptedDEK: base64.StdEncoding.EncodeToString(ct),
	}, nil
}

// Decrypt decrypts the DEK described by m using the KEK identified by m.KEKID.
func (p *KEKEncryptor) Decrypt(ctx context.Context, m *DEKMaterial) (*DEK, error) {
	k, ok := p.keys[m.KEKID]
	if !ok {
		return nil, fmt.Errorf("unknown key: %s", m.KEKID)
	}

	ct, err := base64.StdEncoding.DecodeString(m.EncryptedDEK)
	if err != nil {
		return nil, fmt.Errorf("failed to decode DEK: %s, %w", m.KEKID, err)
	}

	key, err := k.Decrypt(ctx, ct)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt using KEK: %s, %w", m.KEKID, err)
	}

	if len(key) != keyBytes {
		return nil, fmt.Errorf("invalid DEK for KEK: %s", m.KEKID)
	}

	return dekFromKey(key)
}

// Close closes all registered KEKs and releases their resources.
// Subsequent calls return the same error as the first call.
func (p *KEKEncryptor) Close() error {
	// Blocking concurrent callers here is acceptable: Close is a shutdown
	// operation; callers should not race to close, and if they do, waiting
	// for a single authoritative result is the right behaviour.
	p.closeOnce.Do(func() {
		errs := make([]error, 0, len(p.keys)+1)
		for id, kek := range p.keys {
			if err := kek.Close(); err != nil {
				errs = append(errs, fmt.Errorf("failed to close KEK: %s, %w", id, err))
			}
		}

		p.closeErr = errors.Join(errs...)
	})

	return p.closeErr
}

func (k *nilKEK) ID() string                                           { return "EMPTY_KEK" }
func (k *nilKEK) Encrypt(_ context.Context, pt []byte) ([]byte, error) { return pt, nil }
func (k *nilKEK) Decrypt(_ context.Context, ct []byte) ([]byte, error) { return ct, nil }
func (k *nilKEK) Close() error                                         { return nil }
