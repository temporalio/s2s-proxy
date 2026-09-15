package vault

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/temporalio/temporal-proxy/pkg/crypto"
)

// testingKeyScheme addresses a local in-process key. Unlike the cloud KMS
// schemes it carries its own key material inline, which is what
// [safeKeyString] exists to strip.
const testingKeyScheme = "testing://"

// cryptoProviders maps a key URI scheme onto the provider label its metrics
// carry. Schemes may share a label: both local schemes report as "testing", so
// a key that never should have left a developer's laptop is one series on a
// dashboard rather than two. A scheme that is absent here is its own label, so
// a scheme registered on the underlying factory still produces usable metrics
// without being listed.
var cryptoProviders = map[string]string{
	"awskms":        "aws",
	"gcpkms":        "gcp",
	"azurekeyvault": "azure",
	"base64key":     "testing",
	"testing":       "testing",
}

type (
	// KeyFactory opens [crypto.KEK]s and wraps each one so the KMS calls it makes
	// are timed and counted. It adds nothing else: every URI scheme
	// [crypto.KeyFactory] serves works here, and behaves the same way.
	//
	// A KeyFactory never changes after construction, so one may be shared by any
	// number of goroutines opening keys at once.
	KeyFactory struct {
		kf    *crypto.KeyFactory
		meter CryptoMeter
	}

	// cryptoKey is a [crypto.KEK] that reports the duration and outcome of every
	// wrap and unwrap it performs. Only Encrypt and Decrypt are measured; ID and
	// Close come straight from the embedded KEK.
	cryptoKey struct {
		crypto.KEK
		provider string
		meter    CryptoMeter
	}

	// CryptoMeter records KEK operations.
	CryptoMeter interface {
		// Op records one completed KEK operation, where operation is "wrap" or
		// "unwrap", result is "success" or "error", and seconds is how long the call
		// took.
		Op(provider, operation, result string, seconds float64)

		// Observe records a vault event, implementing [crypto.Observer]. An event of a
		// type this meter has no metric for is dropped rather than being an error: the
		// event set belongs to the crypto package and may grow.
		Observe(e crypto.Event)
	}
)

// NewKeyFactory returns a KeyFactory reporting to m the operations of every key
// it opens.
func NewKeyFactory(m CryptoMeter) *KeyFactory {
	return &KeyFactory{
		kf:    crypto.NewKeyFactory(),
		meter: m,
	}
}

// Create opens the key addressed by uri and wraps it so its wrap and unwrap
// calls are measured under the provider label the URI scheme implies (see
// [providerFor]). Which schemes are valid, and what a bad or unreachable key
// looks like, is left to [crypto.KeyFactory.Create].
//
// Close the returned key when it is no longer needed.
func (f *KeyFactory) Create(ctx context.Context, uri string) (crypto.KEK, error) {
	k, err := f.kf.Create(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("error creating KEK: %w", err)
	}

	return &cryptoKey{
		KEK:      k,
		provider: providerFor(uri),
		meter:    f.meter,
	}, nil
}

// Encrypt wraps b for ns, recording the call as a "wrap" against the key's
// provider. The duration is recorded whether the call succeeded or not, so a
// KMS that is timing out shows up as latency and not only as errors.
func (m *cryptoKey) Encrypt(ctx context.Context, ns string, b []byte) ([]byte, error) {
	start := time.Now()
	ct, err := m.KEK.Encrypt(ctx, ns, b)
	m.meter.Op(m.provider, "wrap", resultLabel(err), time.Since(start).Seconds())

	return ct, err
}

// Decrypt unwraps b, recording the call as an "unwrap" against the key's
// provider on the same terms as [cryptoKey.Encrypt].
func (m *cryptoKey) Decrypt(ctx context.Context, b []byte) ([]byte, error) {
	start := time.Now()
	pt, err := m.KEK.Decrypt(ctx, b)
	m.meter.Op(m.provider, "unwrap", resultLabel(err), time.Since(start).Seconds())

	return pt, err
}

// providerFor returns the metrics provider label for a key URI: its scheme, run
// through [cryptoProviders], lowercased because URI schemes are
// case-insensitive and a caller may spell one any way it likes.
//
// The scheme is whatever precedes the first colon. A URI without one is not a
// URI [crypto.KeyFactory.Create] would have opened, so there is nothing to
// guard against here: the whole string becomes the label, and a label from a
// key that never opened never reaches a metric.
func providerFor(uri string) string {
	provider, _, _ := strings.Cut(uri, ":")
	provider = strings.ToLower(provider)
	if p, ok := cryptoProviders[provider]; ok {
		return p
	}

	return provider
}

// resultLabel is the metrics result label for an operation that returned err.
func resultLabel(err error) string {
	if err != nil {
		return "error"
	}

	return "success"
}

// safeKeyString returns uri with the key material a [testingKeyScheme] URI
// carries inline replaced, so the URI is safe to log or report in an error.
// Every other scheme names a key rather than carrying it, and that name is what
// makes a report useful, so those are returned unchanged.
func safeKeyString(uri string) string {
	if len(uri) < len(testingKeyScheme) || !strings.EqualFold(uri[:len(testingKeyScheme)], testingKeyScheme) {
		return uri
	}

	return testingKeyScheme + "<redacted>"
}
