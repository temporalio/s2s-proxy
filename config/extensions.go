package config

import (
	"github.com/temporalio/temporal-proxy/pkg/validation"

	"github.com/temporalio/s2s-proxy/encryption"
)

type (
	// ExtensionServer addresses an operator-run gRPC server implementing
	// api.kms.v1.EncryptionService, the pluggable Key Encryption Key provider.
	// The proxy has built-in KMS providers (awskms, azurekeyvault, gcpkms); an
	// extension server is how an operator plugs in a backend the proxy does not
	// support natively, such as an on-prem HSM or an internal key service.
	//
	// Only key material crosses the wire. The server wraps and unwraps the data
	// encryption keys the proxy seals payloads with, and never sees a payload.
	//
	// Name identifies the server within the configuration so a key URI can
	// reference it as "extension://<name>/<key>", and must be unique across the
	// list.
	//
	// Operators build a server against this contract with temporal-proxy's
	// pkg/ext; see its examples/kms for a worked one.
	ExtensionServer struct {
		Name      string               `yaml:"name"`
		Address   string               `yaml:"address"`
		TLSConfig encryption.TLSConfig `yaml:"tls"`
	}

	// ExtensionServerList is the configured set of extension servers. It exists
	// as a named type so the checks that span the whole collection - name and
	// address uniqueness - live alongside the per-entry checks instead of in the
	// parent config.
	ExtensionServerList []ExtensionServer
)

// Validate checks a single extension server: a name is required and the address
// must be a literal host:port. Failures are unattributed, leaving the caller to
// stamp the path - ExtensionServerList supplies the index.
func (s *ExtensionServer) Validate() error {
	return validation.Validate(
		"",
		validation.Field("name", s.Name, validation.Required[string]()),
		validation.Field("address", s.Address, validation.IsHostPort()),
	)
}

// Validate checks every entry and enforces that names and addresses are unique
// across the list: two servers sharing a name would make a key URI ambiguous and
// would silently collapse into one connection, and two sharing an address is a
// copy-paste error rather than a useful configuration.
//
// Uniqueness failures are reported on a "[name]"/"[address]" field because they
// belong to the collection rather than to any one entry, while per-entry
// failures are stamped with a "[i]" subject. Both compose onto the parent's
// path, so the top-level config surfaces them as "extensionServers[name]" and
// "extensionServers[0]". An empty or nil list is valid.
func (sl ExtensionServerList) Validate() error {
	names := make([]string, len(sl))
	addresses := make([]string, len(sl))
	for i, s := range sl {
		names[i] = s.Name
		addresses[i] = s.Address
	}

	return validation.Validate(
		"",
		validation.Field("[name]", names, validation.Unique[string]()),
		validation.Field("[address]", addresses, validation.Unique[string]()),
		validation.Children("", sl, (*ExtensionServer).Validate),
	)
}

// names is the set of configured server names, for the referential checks that
// resolve "extension://" key URIs.
func (sl ExtensionServerList) names() map[string]struct{} {
	known := make(map[string]struct{}, len(sl))
	for _, s := range sl {
		known[s.Name] = struct{}{}
	}

	return known
}
