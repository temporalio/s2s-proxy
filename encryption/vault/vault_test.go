package vault

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/config"
)

// vaultFixture is a [New] call waiting to happen, holding onto the meter the
// vault reports to so a test can read it afterwards.
type vaultFixture struct {
	cfg   Config
	meter *fakeOpMeter
}

func TestNewVaultRoundTrip(t *testing.T) {
	f := newVaultFixture(encryptionConfig())
	v := requireVault(t, f)

	msg, err := v.Seal(t.Context(), "some-namespace", []byte("payload"))
	require.NoError(t, err)
	require.NotEqual(t, []byte("payload"), msg.Ciphertext)
	require.Equal(t, testingKeyID(1), msg.KeyMaterial.KEKID, "the default policy's key wrapped the DEK")

	plaintext, err := v.Open(t.Context(), msg)
	require.NoError(t, err)
	require.Equal(t, []byte("payload"), plaintext)
}

func TestNewVaultKeySelection(t *testing.T) {
	ec := encryptionConfig()
	ec.Overrides = map[string]config.KeyPolicy{"tenant-a": *keyPolicy(2)}
	v := requireVault(t, newVaultFixture(ec))

	// Each namespace's DEK is wrapped by the key its policy names, and either
	// message opens against the same vault.
	for ns, want := range map[string]string{"tenant-a": testingKeyID(2), "tenant-b": testingKeyID(1)} {
		msg, err := v.Seal(t.Context(), ns, []byte("payload"))
		require.NoError(t, err)
		require.Equal(t, want, msg.KeyMaterial.KEKID, "namespace %s", ns)

		plaintext, err := v.Open(t.Context(), msg)
		require.NoError(t, err)
		require.Equal(t, []byte("payload"), plaintext)
	}
}

func TestNewVaultRotationSchedules(t *testing.T) {
	t.Run("an override rotates on its own schedule", func(t *testing.T) {
		ec := encryptionConfig()
		ec.Overrides = map[string]config.KeyPolicy{"impatient": {URI: testingKeyURI(2), Duration: time.Nanosecond}}

		f := newVaultFixture(ec)
		v := requireVault(t, f)

		sealTwice(t, v, "impatient")
		sealTwice(t, v, "patient")

		// The override's DEK is stale the moment it is made, so every seal replaces
		// it. The other namespace inherits the default policy's hour and needs a
		// first key and nothing more.
		require.Equal(t, []crypto.RotationEvent{
			{Namespace: "impatient", Reason: crypto.RotationOnDemand},
			{Namespace: "impatient", Reason: crypto.RotationOnDemand},
			{Namespace: "patient", Reason: crypto.RotationInitial},
		}, rotations(f.meter))
	})

	t.Run("the default schedule covers namespaces with no override", func(t *testing.T) {
		ec := encryptionConfig()
		ec.Default = &config.KeyPolicy{URI: testingKeyURI(1), Duration: time.Nanosecond}

		f := newVaultFixture(ec)
		sealTwice(t, requireVault(t, f), "some-namespace")

		require.Equal(t, []crypto.RotationEvent{
			{Namespace: "some-namespace", Reason: crypto.RotationInitial},
			{Namespace: "some-namespace", Reason: crypto.RotationOnDemand},
		}, rotations(f.meter))
	})
}

func TestNewVaultCacheSize(t *testing.T) {
	t.Run("a cache spares the KEK a second unwrap", func(t *testing.T) {
		f := newVaultFixture(encryptionConfig())
		openTwice(t, requireVault(t, f))

		require.Equal(t, []crypto.CacheEvent{{Hit: false, Size: 0}, {Hit: true, Size: 1}}, cacheEvents(f.meter))
		require.Equal(t, 1, countOps(f.meter, "unwrap"), "the second open came from the cache")
	})

	t.Run("a zero cache size disables caching", func(t *testing.T) {
		ec := encryptionConfig()
		ec.CacheSize = 0

		f := newVaultFixture(ec)
		openTwice(t, requireVault(t, f))

		require.Empty(t, cacheEvents(f.meter), "there is no cache to report on")
		require.Equal(t, 2, countOps(f.meter, "unwrap"), "every open goes back to the KEK")
	})
}

func TestNewVaultMeter(t *testing.T) {
	t.Run("KEK operations and vault events reach the meter", func(t *testing.T) {
		f := newVaultFixture(encryptionConfig())
		v := requireVault(t, f)

		msg, err := v.Seal(t.Context(), "some-namespace", []byte("payload"))
		require.NoError(t, err)
		_, err = v.Open(t.Context(), msg)
		require.NoError(t, err)

		// The KEK calls are measured by the key wrapper, under the label the
		// testing scheme maps to.
		require.Equal(t, []string{"wrap", "unwrap"}, operations(f.meter))
		for _, op := range f.meter.ops {
			require.Equal(t, "testing", op.provider)
			require.Equal(t, "success", op.result)
		}

		// The vault's own events arrive on the same meter: one envelope event per
		// call, plus the rotation that made the first DEK and the cache miss the
		// open took.
		require.Len(t, envelopes(f.meter), 2)
		require.Len(t, rotations(f.meter), 1)
		require.Len(t, cacheEvents(f.meter), 1)
	})

	t.Run("no logger means the lines are dropped", func(t *testing.T) {
		f := newVaultFixture(encryptionConfig())
		f.cfg.Logger = nil

		// Registering a key logs, so a nil logger that was not filled in would
		// take this down rather than merely staying quiet.
		v := requireVault(t, f)
		_, err := v.Seal(t.Context(), "some-namespace", []byte("payload"))
		require.NoError(t, err)
	})

	t.Run("no meter means the process-wide one", func(t *testing.T) {
		f := newVaultFixture(encryptionConfig())
		f.cfg.Meter = nil

		v := requireVault(t, f)
		msg, err := v.Seal(t.Context(), "some-namespace", []byte("payload"))
		require.NoError(t, err)

		plaintext, err := v.Open(t.Context(), msg)
		require.NoError(t, err)
		require.Equal(t, []byte("payload"), plaintext)
	})
}

func TestVaultClose(t *testing.T) {
	f := newVaultFixture(encryptionConfig())

	v, err := New(t.Context(), f.cfg)
	require.NoError(t, err)
	require.NoError(t, v.Close())

	// Sealing for a namespace that has no DEK yet has to reach the KEK, and there
	// is no longer one to reach.
	_, err = v.Seal(t.Context(), "some-namespace", []byte("payload"))
	require.Error(t, err)

	// Closing again is not an error to report to whoever asked twice; it is the
	// first close's outcome, again.
	require.NoError(t, v.Close())
}

func TestVaultIsACloser(t *testing.T) {
	// The whole point of the type: a caller holds one thing, and that thing knows
	// how to release the keys behind it.
	var _ io.Closer = (*Vault)(nil)
}

func TestNewVaultErrors(t *testing.T) {
	t.Run("an invalid config is reported before any key is opened", func(t *testing.T) {
		ec := encryptionConfig()
		ec.Default.RenewBefore = 2 * ec.Default.Duration

		f := newVaultFixture(ec)
		v, err := New(t.Context(), f.cfg)
		require.Nil(t, v)
		require.ErrorContains(t, err, "invalid encryption config")
	})

	t.Run("a disabled config gets no vault", func(t *testing.T) {
		// The zero config: encryption off and no keys named. Reading a key policy
		// out of it would be a nil dereference, which is the other reason this is
		// the first thing checked.
		f := newVaultFixture(config.EncryptionConfig{})

		v, err := New(t.Context(), f.cfg)
		require.Nil(t, v)
		require.ErrorContains(t, err, "encryption is disabled")
	})

	t.Run("a disabled config gets no vault even when it names keys", func(t *testing.T) {
		// Enabled is the question being asked, not whether a usable key happens to
		// be lying around: a config that turned encryption off does not get a
		// working encrypting vault back.
		ec := encryptionConfig()
		ec.Enabled = false

		v, err := New(t.Context(), newVaultFixture(ec).cfg)
		require.Nil(t, v)
		require.ErrorContains(t, err, "encryption is disabled")
	})

	t.Run("enabling encryption without a default policy is reported", func(t *testing.T) {
		f := newVaultFixture(config.EncryptionConfig{Enabled: true})

		_, err := New(t.Context(), f.cfg)
		require.ErrorContains(t, err, "invalid encryption config")
	})

	t.Run("a key that will not open is reported", func(t *testing.T) {
		ec := encryptionConfig()
		ec.Default = &config.KeyPolicy{URI: "testing://not-valid-base64!!", Duration: time.Hour}

		f := newVaultFixture(ec)
		v, err := New(t.Context(), f.cfg)
		require.Nil(t, v)
		require.ErrorContains(t, err, "error creating KEK")
		require.NotContains(t, err.Error(), "not-valid-base64", "key material stays out of the error")
	})
}

// newVaultFixture returns a fixture whose VaultConfig describes ec and reports
// to fakes the test can inspect afterwards.
func newVaultFixture(ec config.EncryptionConfig) *vaultFixture {
	meter := &fakeOpMeter{}

	return &vaultFixture{
		cfg: Config{
			Logger:     log.NewNoopLogger(),
			Encryption: ec,
			Meter:      meter,
		},
		meter: meter,
	}
}

// requireVault builds f's vault, failing the test if it cannot, and releases
// its keys when the test ends.
func requireVault(t *testing.T, f *vaultFixture) *Vault {
	t.Helper()

	v, err := New(t.Context(), f.cfg)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, v.Close()) })

	return v
}

// encryptionConfig returns a valid config: one key, an hour between rotations,
// and a cache big enough that nothing is evicted mid-test.
func encryptionConfig() config.EncryptionConfig {
	return config.EncryptionConfig{
		Enabled:   true,
		CacheSize: 10,
		Default:   keyPolicy(1),
	}
}

// sealTwice seals two payloads for ns, which is enough to tell a DEK that
// rotates every time from one that is made once and kept.
func sealTwice(t *testing.T, v *Vault, ns string) {
	t.Helper()

	for range 2 {
		_, err := v.Seal(t.Context(), ns, []byte("payload"))
		require.NoError(t, err)
	}
}

// openTwice seals one payload and opens it twice, which is enough to tell a
// warm DEK cache from no cache at all.
func openTwice(t *testing.T, v *Vault) {
	t.Helper()

	msg, err := v.Seal(t.Context(), "some-namespace", []byte("payload"))
	require.NoError(t, err)

	for range 2 {
		plaintext, err := v.Open(t.Context(), msg)
		require.NoError(t, err)
		require.Equal(t, []byte("payload"), plaintext)
	}
}

func rotations(m *fakeOpMeter) []crypto.RotationEvent {
	return eventsOfType[crypto.RotationEvent](m)
}

func cacheEvents(m *fakeOpMeter) []crypto.CacheEvent {
	return eventsOfType[crypto.CacheEvent](m)
}

func envelopes(m *fakeOpMeter) []crypto.EnvelopeEvent {
	return eventsOfType[crypto.EnvelopeEvent](m)
}

// eventsOfType returns the events of type E the meter saw, in order.
func eventsOfType[E crypto.Event](m *fakeOpMeter) []E {
	var out []E
	for _, e := range m.events {
		if ev, ok := e.(E); ok {
			out = append(out, ev)
		}
	}

	return out
}

// operations returns the KEK operations the meter saw, in order.
func operations(m *fakeOpMeter) []string {
	out := make([]string, 0, len(m.ops))
	for _, op := range m.ops {
		out = append(out, op.operation)
	}

	return out
}

// countOps returns how many times the meter saw the named KEK operation.
func countOps(m *fakeOpMeter, operation string) int {
	var n int
	for _, op := range m.ops {
		if op.operation == operation {
			n++
		}
	}

	return n
}
