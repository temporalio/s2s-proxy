package extension

import (
	"context"
	"fmt"
	"net/url"

	"github.com/temporalio/temporal-proxy/pkg/crypto"

	"github.com/temporalio/s2s-proxy/config"
)

// Scheme is the key URI scheme an extension server serves, as
// "extension://<server>/<key>". It is the config's scheme so the name an
// operator writes and the name resolved here cannot drift apart.
const Scheme = config.ExtensionKeyScheme

// NewKeyFunc returns the opener for [Scheme] URIs, resolving each to a KMS
// client on the named server's connection.
//
// Several keys may share one server, so the whole URI is the key's identity
// rather than the server name: it is recorded in every DEK the key wraps and is
// what selects the key again on the decrypt path. The identity is the parsed
// URI, so a key opened as "EXTENSION://..." matches the same key spelled in
// lower case. Respelling a configured URI in any other way mints a new identity
// and orphans payloads sealed under the old one.
//
// conns is captured rather than copied, and is looked up when a key is opened
// rather than here, so a URI naming a server that is absent fails at that point
// and not at construction.
func NewKeyFunc(conns Connections) crypto.KeyFactoryFunc {
	return func(_ context.Context, uri string) (crypto.KEK, error) {
		// Unreachable as registered, since the crypto factory parses a URI before
		// dispatching on its scheme. It is handled anyway because KeyFactoryFunc is
		// a public contract and nothing stops a caller invoking this directly.
		u, err := url.Parse(uri)
		if err != nil {
			return nil, fmt.Errorf("failed to parse key URI: %s, %w", uri, err)
		}

		if u.Host == "" {
			return nil, fmt.Errorf("extension key URI must name an extension server: %s", uri)
		}

		conn, ok := conns[u.Host]
		if !ok {
			return nil, fmt.Errorf("unknown extension server %q in key URI: %s", u.Host, uri)
		}

		return NewKMS(u.String(), conn), nil
	}
}
