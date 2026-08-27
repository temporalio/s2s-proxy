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

var validKeySchemes = crypto.DefaultSchemes()

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

		return nil
	}
}

func validKeyURIRef() validation.Check[*string] {
	check := validKeyURI()
	return func(raw *string) error {
		return check(*raw)
	}
}
