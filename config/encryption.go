package config

import (
	"fmt"
	"maps"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/temporalio/temporal-proxy/pkg/crypto"
	"github.com/temporalio/temporal-proxy/pkg/validation"
)

// ExtensionKeyScheme addresses a key served by a configured extension server,
// as "extension://<server>/<key>". The host names an entry in ExtensionServers;
// the key is a proxy-side identifier distinguishing several keys hosted by one
// server, and is never sent to the server, which selects keys by namespace when
// wrapping and reads the key back out of its own ciphertext when unwrapping.
const ExtensionKeyScheme = "extension"

// validKeySchemes is every scheme crypto opens by default plus the extension
// scheme this proxy resolves itself. DefaultSchemes returns a fresh slice per
// call, so appending to it here is safe.
var validKeySchemes = append(crypto.DefaultSchemes(), ExtensionKeyScheme)

type (
	// EncryptionConfig configures envelope encryption of replication payloads.
	// Payloads are sealed with a data encryption key (DEK), which is itself
	// wrapped by a key encryption key (KEK) held in a cloud KMS. Default and
	// Overrides are validated whether or not Enabled is set, so a broken policy
	// gets reported before someone switches it on.
	EncryptionConfig struct {
		// Turn on envelope encryption, which requires Default to be set
		Enabled bool `yaml:"enabled"`
		// Maximum number of unwrapped DEKs to hold in memory, or 0 to disable caching
		CacheSize int `yaml:"cacheSize"`
		// Key policy for namespaces with no entry in Overrides
		Default *KeyPolicy `yaml:"default"`
		// Per-namespace key policies, keyed by namespace name, replacing Default
		Overrides map[string]KeyPolicy `yaml:"overrides,omitempty"`
	}

	// KeyPolicy names the KEK that wraps a namespace's DEKs and sets how often
	// those DEKs rotate. It mirrors crypto.KeyConfig, including its bounds.
	KeyPolicy struct {
		// KMS URI of the key that wraps new DEKs, e.g. awskms://alias/my-key.
		// The scheme picks the provider: awskms, azurekeyvault, gcpkms, or testing
		URI string `yaml:"uri"`
		// Extra key URIs accepted when unwrapping, never chosen for new DEKs.
		// Retired keys belong here so payloads sealed with them stay readable
		DecryptURIs []string `yaml:"decryptURIs,omitempty"`
		// How long a DEK may be used before it has to rotate; must be positive
		Duration time.Duration `yaml:"duration"`
		// How far ahead of Duration to rotate a DEK; must fall in [0, Duration)
		RenewBefore time.Duration `yaml:"renewBefore"`
	}
)

func (e *EncryptionConfig) Validate() error {
	rules := []validation.Rule{
		validation.Field("cacheSize", e.CacheSize, validation.GTE(0)),
		validation.WhenRules(
			func() bool { return e.Enabled },
			validation.Field("default", e.Default, validation.Required[*KeyPolicy]()),
		),
		validation.WhenNested(func() bool { return e.Default != nil }, "default", e.Default),
	}

	// Sort the namespace keys so error ordering is deterministic across runs.
	for _, ns := range slices.Sorted(maps.Keys(e.Overrides)) {
		policy := e.Overrides[ns]
		subject := fmt.Sprintf("overrides[%s]", ns)
		rules = append(rules,
			validation.Field(subject, ns, validation.Required[string]()),
			validation.Nested(subject, &policy),
		)
	}

	return validation.Validate("", rules...)
}

func (p *KeyPolicy) Validate() error {
	var zd time.Duration

	return validation.Validate(
		"",
		validation.Field("uri", p.URI, validKeyURI()),
		validation.Children("decryptURIs", p.DecryptURIs, validKeyURIRef()),
		validation.Field("duration", p.Duration, validation.GT(zd)),
		validation.Field("renewBefore", p.RenewBefore, validation.GTE(zd), validation.LT(p.Duration)),
	)
}

func validKeyURI() validation.Check[string] {
	return func(raw string) error {
		u, err := url.Parse(raw)
		if err != nil {
			return fmt.Errorf("is not a valid URI: %w", err)
		}

		if !slices.ContainsFunc(validKeySchemes, func(s string) bool {
			return strings.EqualFold(s, u.Scheme)
		}) {
			return fmt.Errorf(
				"invalid key URI: %s, valid schemes: [%s]",
				raw,
				strings.Join(validKeySchemes, ","),
			)
		}

		// An extension URI references a configured server by host, so a missing
		// one is checked here rather than by the referential rules: those report a
		// host matching no configured server, and an empty host gives them no name
		// to report.
		if strings.EqualFold(u.Scheme, ExtensionKeyScheme) && u.Host == "" {
			return fmt.Errorf("extension key URI must name an extension server: %s", raw)
		}

		return nil
	}
}

func validKeyURIRef() validation.Check[*string] {
	check := validKeyURI()
	return func(raw *string) error {
		return check(*raw)
	}
}

// referentialRules checks that every "extension://" key URI names a configured
// extension server, given the set of known names. Each failure is stamped with
// the referring policy's YAML path, so prefix is the path of the encryption
// block itself (e.g. "clusterConnections[0].encryption") and the rules extend it
// to "...encryption.default"/"uri" or "...encryption.overrides[payments]".
//
// These rules live apart from Validate because they need the full set of
// extension server names, which only the top-level config knows. Keeping them
// out also keeps Validate structural, so vault.New can go on calling it without
// a server set to hand over.
//
// Matching is case-sensitive, matching the lookup the key factory does when it
// opens the key, so a name that validates is a name that resolves.
func (e *EncryptionConfig) referentialRules(prefix string, known map[string]struct{}) []validation.Rule {
	var rules []validation.Rule

	policy := func(subject string, p *KeyPolicy) {
		rules = append(rules, extensionRef(subject, "uri", p.URI, known))
		for i, uri := range p.DecryptURIs {
			rules = append(rules, extensionRef(subject, fmt.Sprintf("decryptURIs[%d]", i), uri, known))
		}
	}

	if e.Default != nil {
		policy(prefix+".default", e.Default)
	}

	// Sorted so error ordering is deterministic across runs, matching Validate.
	for _, ns := range slices.Sorted(maps.Keys(e.Overrides)) {
		p := e.Overrides[ns]
		policy(fmt.Sprintf("%s.overrides[%s]", prefix, ns), &p)
	}

	return rules
}

// extensionRef builds a Rule reporting an extension key URI whose host names no
// configured extension server. A URI with another scheme, an unparseable one, or
// one with no host at all yields nothing: the first is not a reference, and the
// other two are already reported by validKeyURI.
func extensionRef(subject, field, raw string, known map[string]struct{}) validation.Rule {
	return func() validation.Errors {
		u, err := url.Parse(raw)
		if err != nil || !strings.EqualFold(u.Scheme, ExtensionKeyScheme) || u.Host == "" {
			return nil
		}

		if _, ok := known[u.Host]; ok {
			return nil
		}

		return validation.Errors{{
			Subject: subject,
			Field:   field,
			Message: fmt.Sprintf("unknown extension server: %s", u.Host),
		}}
	}
}
