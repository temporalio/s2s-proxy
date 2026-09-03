package vault

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/config"
)

type (
	// registryFixture is a [createRegistry] call waiting to happen, with fakes in
	// place of what the real thing would reach: the log and the metrics.
	registryFixture struct {
		opts   registryConfig
		logger *recordingLogger
		meter  *fakeOpMeter
	}

	// fakeKeyFactory opens stub keys instead of real ones, so a test can see
	// which keys were opened and whether they were closed again. A URI named in
	// fails never opens.
	fakeKeyFactory struct {
		fails map[string]error
		keys  []*fakeKEK
	}

	// fakeKEK counts the times it is closed. A nil embedded KEK means anything
	// beyond ID and Close panics rather than quietly working.
	fakeKEK struct {
		crypto.KEK

		id     string
		closes int
	}

	// recordingLogger captures what was logged to it. A nil embedded Logger means
	// a level nothing under test uses panics rather than being quietly dropped.
	recordingLogger struct {
		log.Logger

		entries []logEntry
	}

	logEntry struct {
		msg  string
		tags map[string]any
	}
)

func TestCreateRegistryKeySelection(t *testing.T) {
	t.Run("the default policy's key serves every namespace", func(t *testing.T) {
		f := newRegistryFixture(config.EncryptionConfig{Default: keyPolicy(1)})
		r := requireRegistry(t, f)

		// No namespace is registered, so both fall through to the default key.
		require.Equal(t, testingKeyID(1), sealDEK(t, r, "some-namespace").KEKID)
		require.Equal(t, testingKeyID(1), sealDEK(t, r, "another-namespace").KEKID)
	})

	t.Run("an override's key serves its own namespace only", func(t *testing.T) {
		f := newRegistryFixture(config.EncryptionConfig{
			Default:   keyPolicy(1),
			Overrides: map[string]config.KeyPolicy{"tenant-a": *keyPolicy(2)},
		})
		r := requireRegistry(t, f)

		require.Equal(t, testingKeyID(2), sealDEK(t, r, "tenant-a").KEKID)
		require.Equal(t, testingKeyID(1), sealDEK(t, r, "tenant-b").KEKID)
	})

	t.Run("an override named default is a namespace, not the default key", func(t *testing.T) {
		// The default policy is matched by pointer, not by the "default" label it
		// is logged under, so a namespace that happens to be called that is
		// registered like any other.
		f := newRegistryFixture(config.EncryptionConfig{
			Default:   keyPolicy(1),
			Overrides: map[string]config.KeyPolicy{"default": *keyPolicy(2)},
		})
		r := requireRegistry(t, f)

		require.Equal(t, testingKeyID(2), sealDEK(t, r, "default").KEKID)
		require.Equal(t, testingKeyID(1), sealDEK(t, r, "anything-else").KEKID)
	})

	t.Run("a decrypt URI opens old payloads but never seals new ones", func(t *testing.T) {
		policy := keyPolicy(1)
		policy.DecryptURIs = []string{testingKeyURI(2)}

		f := newRegistryFixture(config.EncryptionConfig{Default: policy})
		r := requireRegistry(t, f)

		// Seal a payload with the retired key by way of a registry that still
		// considers it current, then hand the material to the registry under test.
		retired := requireRegistry(t, newRegistryFixture(config.EncryptionConfig{Default: keyPolicy(2)}))
		dek, err := crypto.NewDEK()
		require.NoError(t, err)
		material, err := retired.Encrypt(t.Context(), "some-namespace", dek)
		require.NoError(t, err)
		ciphertext, err := dek.Encrypt(t.Context(), []byte("payload"))
		require.NoError(t, err)

		recovered, err := r.Decrypt(t.Context(), material)
		require.NoError(t, err)
		plaintext, err := recovered.Decrypt(t.Context(), ciphertext)
		require.NoError(t, err)
		require.Equal(t, []byte("payload"), plaintext)

		// New DEKs still go to the policy's own key.
		require.Equal(t, testingKeyID(1), sealDEK(t, r, "some-namespace").KEKID)
	})
}

func TestCreateRegistryReleasesKeysWhenItFails(t *testing.T) {
	// No registry comes back from these, so nothing else can close what was
	// opened on the way. Every key has to be released here or it is leaked for
	// the life of the process.
	cases := []struct {
		name      string
		ec        config.EncryptionConfig
		wantKeys  int
		wantError string
	}{
		{
			name: "a namespace after the first fails",
			ec: config.EncryptionConfig{
				Default:   &config.KeyPolicy{URI: "good-default"},
				Overrides: map[string]config.KeyPolicy{"tenant-a": {URI: "broken"}},
			},
			wantKeys:  1,
			wantError: "boom",
		},
		{
			name: "a decrypt URI fails after the policy's own key",
			ec: config.EncryptionConfig{
				Default: &config.KeyPolicy{URI: "good-default", DecryptURIs: []string{"broken"}},
			},
			wantKeys:  1,
			wantError: "boom",
		},
		{
			name: "the registry rejects the keys it was handed",
			ec: config.EncryptionConfig{
				Default:   &config.KeyPolicy{URI: "good-default"},
				Overrides: map[string]config.KeyPolicy{"tenant-a": {URI: "good-default"}},
			},
			wantKeys:  2,
			wantError: "duplicate key id",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kf := &fakeKeyFactory{fails: map[string]error{"broken": errors.New("boom")}}

			r, err := createRegistry(t.Context(), registryConfig{
				log: &recordingLogger{},
				ec:  tc.ec,
				kf:  kf,
			})
			require.Nil(t, r)
			require.ErrorContains(t, err, tc.wantError)

			require.Len(t, kf.keys, tc.wantKeys, "keys opened before the failure")
			for _, k := range kf.keys {
				require.Equal(t, 1, k.closes, "key %s was released exactly once", k.id)
			}
		})
	}
}

func TestCreateRegistryKeepsItsKeysOnSuccess(t *testing.T) {
	kf := &fakeKeyFactory{}

	r, err := createRegistry(t.Context(), registryConfig{
		log: &recordingLogger{},
		ec:  config.EncryptionConfig{Default: &config.KeyPolicy{URI: "good-default"}},
		kf:  kf,
	})
	require.NoError(t, err)

	require.Len(t, kf.keys, 1)
	require.Zero(t, kf.keys[0].closes, "the registry owns the key until its owner closes it")

	require.NoError(t, r.Close())
	require.Equal(t, 1, kf.keys[0].closes)
}

func TestCreateRegistryErrors(t *testing.T) {
	t.Run("a default key that will not open", func(t *testing.T) {
		f := newRegistryFixture(config.EncryptionConfig{
			Default: &config.KeyPolicy{URI: "testing://not-valid-base64!!"},
		})

		r, err := createRegistry(t.Context(), f.opts)
		require.Nil(t, r)
		require.ErrorContains(t, err, "error creating KEK")
		require.NotContains(t, err.Error(), "not-valid-base64", "key material stays out of the error")
	})

	t.Run("a decrypt URI that will not open", func(t *testing.T) {
		policy := keyPolicy(1)
		policy.DecryptURIs = []string{"vault://secret/key"}
		f := newRegistryFixture(config.EncryptionConfig{Default: policy})

		_, err := createRegistry(t.Context(), f.opts)
		require.ErrorContains(t, err, "key factory not found for scheme: vault")
	})

	t.Run("an override key that will not open", func(t *testing.T) {
		f := newRegistryFixture(config.EncryptionConfig{
			Default:   keyPolicy(1),
			Overrides: map[string]config.KeyPolicy{"tenant-a": {URI: "vault://secret/key"}},
		})

		_, err := createRegistry(t.Context(), f.opts)
		require.ErrorContains(t, err, "key factory not found for scheme: vault")
	})

	t.Run("two keys with the same URI collide", func(t *testing.T) {
		// One key URI serves one namespace, by design. The registry indexes keys by
		// ID and two KEKs opened from the same URI report the same one, so sharing
		// a URI is rejected at startup rather than at the first wrap.
		f := newRegistryFixture(config.EncryptionConfig{
			Default:   keyPolicy(1),
			Overrides: map[string]config.KeyPolicy{"tenant-a": *keyPolicy(1)},
		})

		_, err := createRegistry(t.Context(), f.opts)
		require.ErrorContains(t, err, "failed to create KEK registry: duplicate key id")
	})

	t.Run("overrides are visited in namespace order", func(t *testing.T) {
		// Both namespaces are broken, so whichever is reported first is the one
		// that was tried first. Map iteration would make that a coin toss.
		f := newRegistryFixture(config.EncryptionConfig{
			Default: keyPolicy(1),
			Overrides: map[string]config.KeyPolicy{
				"zeta":  {URI: "zeta://secret/key"},
				"alpha": {URI: "alpha://secret/key"},
			},
		})

		_, err := createRegistry(t.Context(), f.opts)
		require.ErrorContains(t, err, "key factory not found for scheme: alpha")
	})
}

func TestCreateRegistryLogsEachKey(t *testing.T) {
	policy := keyPolicy(1)
	policy.DecryptURIs = []string{testingKeyURI(2)}

	f := newRegistryFixture(config.EncryptionConfig{
		Default:   policy,
		Overrides: map[string]config.KeyPolicy{"tenant-a": *keyPolicy(3)},
	})
	requireRegistry(t, f)

	// Every key is announced, decrypt-only keys included, under the namespace it
	// serves. The URI is redacted: a testing key carries its material inline and
	// the log is the last place that should end up.
	require.Equal(t, []logEntry{
		{"Registering crypto key", map[string]any{"namespace": "default", "uri": "testing://<redacted>"}},
		{"Registering crypto key", map[string]any{"namespace": "default", "uri": "testing://<redacted>"}},
		{"Registering crypto key", map[string]any{"namespace": "tenant-a", "uri": "testing://<redacted>"}},
	}, f.logger.entries)
}

func TestCreateRegistryLogsOtherKeysInFull(t *testing.T) {
	// Every other scheme names a key rather than carrying one, and that name is
	// what makes the log line worth having. This key never opens, which is the
	// other half of the point: a key is announced before it is opened, so the one
	// that fails is named right above the failure.
	f := newRegistryFixture(config.EncryptionConfig{
		Default: &config.KeyPolicy{URI: "vault://secret/replication-key"},
	})

	_, err := createRegistry(t.Context(), f.opts)
	require.Error(t, err)
	require.Equal(t, []logEntry{
		{"Registering crypto key", map[string]any{"namespace": "default", "uri": "vault://secret/replication-key"}},
	}, f.logger.entries)
}

// newRegistryFixture returns a fixture whose registryConfig describe ec and
// report to fakes the test can inspect afterwards.
func newRegistryFixture(ec config.EncryptionConfig) *registryFixture {
	logger, meter := &recordingLogger{}, &fakeOpMeter{}

	return &registryFixture{
		opts:   registryConfig{log: logger, ec: ec, kf: NewKeyFactory(meter)},
		logger: logger,
		meter:  meter,
	}
}

// requireRegistry builds f's registry, failing the test if it cannot, and
// closes it when the test ends.
func requireRegistry(t *testing.T, f *registryFixture) *crypto.KEKRegistry {
	t.Helper()

	r, err := createRegistry(t.Context(), f.opts)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, r.Close()) })

	return r
}

// sealDEK wraps a fresh DEK for ns, returning the material naming the KEK that
// was chosen for it.
func sealDEK(t *testing.T, r *crypto.KEKRegistry, ns string) *crypto.DEKMaterial {
	t.Helper()

	dek, err := crypto.NewDEK()
	require.NoError(t, err)

	m, err := r.Encrypt(t.Context(), ns, dek)
	require.NoError(t, err)

	return m
}

// keyPolicy returns a valid policy naming the testing key b, with a rotation
// window long enough that nothing rotates mid-test.
func keyPolicy(b byte) *config.KeyPolicy {
	return &config.KeyPolicy{
		URI:         testingKeyURI(b),
		Duration:    time.Hour,
		RenewBefore: time.Minute,
	}
}

// testingKeyURI builds a "testing://" URI carrying a fixed 32-byte key made of
// b, so each b is a distinguishable key.
func testingKeyURI(b byte) string {
	return testingKeyScheme + testingKeyMaterial(b)
}

// testingKeyID is the ID a key opened from [testingKeyURI] reports, and so the
// KEKID that naming it in a DEK's material looks like.
func testingKeyID(b byte) string {
	return "base64key://" + testingKeyMaterial(b)
}

func testingKeyMaterial(b byte) string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{b}, 32))
}

func (f *fakeKeyFactory) Create(_ context.Context, uri string) (crypto.KEK, error) {
	if err, ok := f.fails[uri]; ok {
		return nil, err
	}

	k := &fakeKEK{id: uri}
	f.keys = append(f.keys, k)

	return k, nil
}

func (k *fakeKEK) ID() string { return k.id }

func (k *fakeKEK) Close() error {
	k.closes++
	return nil
}

func (l *recordingLogger) Info(msg string, tags ...tag.Tag) {
	entry := logEntry{msg: msg, tags: make(map[string]any, len(tags))}
	for _, t := range tags {
		entry.tags[t.Key()] = t.Value()
	}

	l.entries = append(l.entries, entry)
}
