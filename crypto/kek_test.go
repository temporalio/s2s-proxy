package crypto_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/crypto"
)

type fakeKEK struct {
	id         string
	closeCount int
	closeErr   error
	encErr     error
	decErr     error
	decResult  []byte // when non-nil, returned from Decrypt instead of ct
}

func TestKEKEncryptor_Encrypt(t *testing.T) {
	t.Parallel()

	dek, err := crypto.NewDEK()
	require.NoError(t, err)

	tests := []struct {
		name      string
		opts      []crypto.KEKEncryptorOption
		wantErr   bool
		wantKEKID string
	}{
		{
			name:      "without default uses nilKEK",
			wantKEKID: "EMPTY_KEK",
		},
		{
			name:      "",
			opts:      []crypto.KEKEncryptorOption{crypto.WithKey(&fakeKEK{id: "default"})},
			wantKEKID: "default",
		},
		{
			name:    "kek encrypt error",
			opts:    []crypto.KEKEncryptorOption{crypto.WithKey(&fakeKEK{id: "k1", encErr: errors.New("kms unavailable")})},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := crypto.NewKEKEncryptor(tc.opts...)
			m, err := r.Encrypt(t.Context(), dek)
			if tc.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.wantKEKID, m.KEKID)
			require.NotEmpty(t, m.EncryptedDEK)
		})
	}
}

func TestKEKEncryptor_Decrypt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		material *crypto.DEKMaterial
		opts     []crypto.KEKEncryptorOption
		wantErr  bool
	}{
		{
			name:     "unknown key id",
			material: &crypto.DEKMaterial{KEKID: "missing"},
			wantErr:  true,
		},
		{
			name:     "invalid base64",
			material: &crypto.DEKMaterial{KEKID: "k1", EncryptedDEK: "not-valid-base64!!!"},
			opts:     []crypto.KEKEncryptorOption{crypto.WithKey(&fakeKEK{id: "k1"})},
			wantErr:  true,
		},
		{
			name:     "kek decrypt error",
			material: &crypto.DEKMaterial{KEKID: "k1", EncryptedDEK: "AAAA"},
			opts:     []crypto.KEKEncryptorOption{crypto.WithKey(&fakeKEK{id: "k1", decErr: errors.New("kms unavailable")})},
			wantErr:  true,
		},
		{
			name:     "wrong-length dek",
			material: &crypto.DEKMaterial{KEKID: "k1", EncryptedDEK: "AAAA"},
			opts:     []crypto.KEKEncryptorOption{crypto.WithKey(&fakeKEK{id: "k1", decResult: []byte("too short")})},
			wantErr:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			r := crypto.NewKEKEncryptor(tc.opts...)
			_, err := r.Decrypt(t.Context(), tc.material)
			require.Error(t, err)
		})
	}
}

func TestKEKEncryptor_RetiredKeyDecrypt(t *testing.T) {
	t.Parallel()

	original, err := crypto.NewDEK()
	require.NoError(t, err)

	active := &fakeKEK{id: "active"}
	retired := &fakeKEK{id: "retired"}

	// Encrypt with the retired key directly (simulating a DEK from before key rotation).
	r := crypto.NewKEKEncryptor(crypto.WithKey(retired))
	m, err := r.Encrypt(t.Context(), original)
	require.NoError(t, err)
	require.Equal(t, "retired", m.KEKID)

	// New registry: active key for encryption, retired key for decryption only.
	r2 := crypto.NewKEKEncryptor(
		crypto.WithKey(active),
		crypto.WithRetiredKey(retired),
	)

	t.Run("new DEKs use active key", func(t *testing.T) {
		t.Parallel()

		m2, err := r2.Encrypt(t.Context(), original)
		require.NoError(t, err)
		require.Equal(t, "active", m2.KEKID)
	})

	t.Run("old DEKs decrypt via retired key", func(t *testing.T) {
		t.Parallel()

		recovered, err := r2.Decrypt(t.Context(), m)
		require.NoError(t, err)
		require.Equal(t, original, recovered)
	})
}

func TestKEKEncryptor_Close(t *testing.T) {
	t.Parallel()

	t.Run("closes all keks", func(t *testing.T) {
		t.Parallel()

		k1 := &fakeKEK{id: "k1"}
		k2 := &fakeKEK{id: "k2"}
		r := crypto.NewKEKEncryptor(
			crypto.WithKey(k1),
			crypto.WithRetiredKey(k2),
		)

		require.NoError(t, r.Close())
		require.Equal(t, 1, k1.closeCount)
		require.Equal(t, 1, k2.closeCount)
	})

	t.Run("returns error on close failure", func(t *testing.T) {
		t.Parallel()

		kek := &fakeKEK{id: "k1", closeErr: errors.New("close failed")}
		r := crypto.NewKEKEncryptor(crypto.WithKey(kek))
		require.Error(t, r.Close())
	})

	t.Run("idempotent - same result on repeated close", func(t *testing.T) {
		t.Parallel()

		kek := &fakeKEK{id: "k1", closeErr: errors.New("close failed")}
		r := crypto.NewKEKEncryptor(crypto.WithKey(kek))

		err1 := r.Close()
		err2 := r.Close()
		require.Equal(t, err1, err2)
		require.Equal(t, 1, kek.closeCount)
	})

	t.Run("closes retired keys", func(t *testing.T) {
		t.Parallel()

		active := &fakeKEK{id: "active"}
		retired := &fakeKEK{id: "retired"}
		r := crypto.NewKEKEncryptor(
			crypto.WithKey(active),
			crypto.WithRetiredKey(retired),
		)

		require.NoError(t, r.Close())
		require.Equal(t, 1, active.closeCount)
		require.Equal(t, 1, retired.closeCount)
	})

	t.Run("concurrent close calls kek once", func(t *testing.T) {
		t.Parallel()

		kek := &fakeKEK{id: "k1"}
		r := crypto.NewKEKEncryptor(crypto.WithKey(kek))

		errs := make([]error, 20)
		var wg sync.WaitGroup
		for i := range len(errs) {
			wg.Go(func() {
				errs[i] = r.Close()
			})
		}

		wg.Wait()
		require.Equal(t, 1, kek.closeCount)

		for _, err := range errs {
			require.NoError(t, err)
		}
	})
}

func (f *fakeKEK) ID() string { return f.id }

func (f *fakeKEK) Encrypt(_ context.Context, pt []byte) ([]byte, error) {
	if f.encErr != nil {
		return nil, f.encErr
	}

	return pt, nil
}

func (f *fakeKEK) Decrypt(_ context.Context, ct []byte) ([]byte, error) {
	if f.decResult != nil || f.decErr != nil {
		return f.decResult, f.decErr
	}

	return ct, nil
}

func (f *fakeKEK) Close() error {
	f.closeCount++
	return f.closeErr
}
