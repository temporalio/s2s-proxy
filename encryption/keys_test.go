package encryption

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/crypto"
)

type (
	// fakeOpMeter records the KEK operations reported to it, in order.
	fakeOpMeter struct {
		ops []recordedOp
	}

	recordedOp struct {
		provider  string
		operation string
		result    string
		seconds   float64
	}

	stubKEK struct {
		// A nil embedded KEK means any method these tests do not exercise panics
		// rather than quietly returning a zero value.
		crypto.KEK

		gotNS string
		gotIn []byte
		out   []byte
		err   error
	}
)

func TestProviderFor(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want string
	}{
		{"aws", "awskms://alias/replication", "aws"},
		{"gcp", "gcpkms://projects/p/locations/l/keyRings/r/cryptoKeys/k", "gcp"},
		{"azure", "azurekeyvault://vault.vault.azure.net/keys/k", "azure"},
		{"testing", "testing://", "testing"},
		{"testing with material", "testing://c2Vjcg==", "testing"},
		{"base64key shares the testing label", "base64key://c2Vjcg==", "testing"},
		{"scheme match is case insensitive", "AWSKMS://alias/replication", "aws"},
		{"unmapped scheme is its own label", "vault://secret/key", "vault"},
		{"unmapped scheme is lowercased", "VAULT://secret/key", "vault"},
		{"no scheme yields the whole string", "not-a-uri", "not-a-uri"},
		{"empty URI yields an empty label", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, providerFor(tc.uri))
		})
	}
}

func TestKeyFactoryCreate(t *testing.T) {
	t.Run("opens a key and measures it", func(t *testing.T) {
		meter := &fakeOpMeter{}
		k, err := NewKeyFactory(meter).Create(t.Context(), "testing://")
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, k.Close()) })

		// The wrapper is transparent apart from the metering: the ID is the
		// underlying key's, which is what the decrypt path looks a KEK up by.
		require.Equal(t, "base64key://", k.ID())
		require.Empty(t, meter.ops, "opening a key is not itself a KEK operation")
	})

	t.Run("reports an unopenable key", func(t *testing.T) {
		k, err := NewKeyFactory(&fakeOpMeter{}).Create(t.Context(), "vault://secret/key")
		require.Nil(t, k)
		require.ErrorContains(t, err, "error creating KEK: key factory not found for scheme: vault")
	})
}

func TestCryptoKeyMeasuresOperations(t *testing.T) {
	boom := errors.New("kms unavailable")

	newKey := func(stub *stubKEK) (*cryptoKey, *fakeOpMeter) {
		meter := &fakeOpMeter{}
		return &cryptoKey{KEK: stub, provider: "aws", meter: meter}, meter
	}

	t.Run("a successful wrap", func(t *testing.T) {
		stub := &stubKEK{out: []byte("wrapped")}
		k, meter := newKey(stub)

		ct, err := k.Encrypt(t.Context(), "namespace", []byte("dek"))
		require.NoError(t, err)
		require.Equal(t, []byte("wrapped"), ct)
		require.Equal(t, "namespace", stub.gotNS, "the namespace reaches the underlying key")
		require.Equal(t, []byte("dek"), stub.gotIn)
		requireOp(t, meter, "aws", "wrap", "success")
	})

	t.Run("a failed wrap is still measured", func(t *testing.T) {
		k, meter := newKey(&stubKEK{err: boom})

		_, err := k.Encrypt(t.Context(), "namespace", []byte("dek"))
		require.ErrorIs(t, err, boom, "the underlying error is returned as-is")
		requireOp(t, meter, "aws", "wrap", "error")
	})

	t.Run("a successful unwrap", func(t *testing.T) {
		stub := &stubKEK{out: []byte("dek")}
		k, meter := newKey(stub)

		pt, err := k.Decrypt(t.Context(), []byte("wrapped"))
		require.NoError(t, err)
		require.Equal(t, []byte("dek"), pt)
		require.Equal(t, []byte("wrapped"), stub.gotIn)
		requireOp(t, meter, "aws", "unwrap", "success")
	})

	t.Run("a failed unwrap is still measured", func(t *testing.T) {
		k, meter := newKey(&stubKEK{err: boom})

		_, err := k.Decrypt(t.Context(), []byte("wrapped"))
		require.ErrorIs(t, err, boom)
		requireOp(t, meter, "aws", "unwrap", "error")
	})
}

func TestCryptoKeyRoundTrip(t *testing.T) {
	meter := &fakeOpMeter{}
	k, err := NewKeyFactory(meter).Create(t.Context(), "TESTING://")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, k.Close()) })

	ct, err := k.Encrypt(t.Context(), "namespace", []byte("dek"))
	require.NoError(t, err)

	pt, err := k.Decrypt(t.Context(), ct)
	require.NoError(t, err)
	require.Equal(t, []byte("dek"), pt)

	// The provider comes from the URI scheme, however it was spelled, and both
	// halves of the round trip are reported separately.
	require.Len(t, meter.ops, 2)
	require.Equal(t, "testing", meter.ops[0].provider)
	require.Equal(t, "wrap", meter.ops[0].operation)
	require.Equal(t, "success", meter.ops[0].result)
	require.Equal(t, "testing", meter.ops[1].provider)
	require.Equal(t, "unwrap", meter.ops[1].operation)
	require.Equal(t, "success", meter.ops[1].result)
}

func TestSafeKeyString(t *testing.T) {
	cases := []struct {
		name string
		uri  string
		want string
	}{
		{"inline key material is stripped", "testing://c2VjcmV0", "testing://<redacted>"},
		{"scheme match is case insensitive", "TESTING://c2VjcmV0", "testing://<redacted>"},
		{"a bare testing key is redacted too", "testing://", "testing://<redacted>"},
		{"an AWS key names a key rather than carrying one", "awskms://alias/replication", "awskms://alias/replication"},
		{"a GCP key is unchanged", "gcpkms://projects/p/locations/l", "gcpkms://projects/p/locations/l"},
		{"a truncated scheme is unchanged", "testing:", "testing:"},
		{"an empty URI is unchanged", "", ""},
		// A testing key's ID is its material behind gocloud's own scheme, and only
		// the scheme a caller writes is redacted, not that one.
		{"base64key is not redacted", "base64key://c2VjcmV0", "base64key://c2VjcmV0"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, safeKeyString(tc.uri))
		})
	}
}

func (m *fakeOpMeter) Op(provider, operation, result string, seconds float64) {
	m.ops = append(m.ops, recordedOp{provider, operation, result, seconds})
}

// requireOp asserts that meter recorded exactly one operation, matching the
// given labels and carrying a usable duration.
func requireOp(t *testing.T, meter *fakeOpMeter, provider, operation, result string) {
	t.Helper()

	require.Len(t, meter.ops, 1)
	require.Equal(t, recordedOp{provider, operation, result, meter.ops[0].seconds}, meter.ops[0])
	require.GreaterOrEqual(t, meter.ops[0].seconds, 0.0, "a duration is always recorded")
}

func (k *stubKEK) Encrypt(_ context.Context, ns string, dek []byte) ([]byte, error) {
	k.gotNS, k.gotIn = ns, dek
	return k.out, k.err
}

func (k *stubKEK) Decrypt(_ context.Context, dek []byte) ([]byte, error) {
	k.gotIn = dek
	return k.out, k.err
}
