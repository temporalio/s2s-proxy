package config

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

// The set of valid KMS key schemes
var validKeySchemes = []string{
	"awskms",
	"azurekeyvault",
	"gcpkms",
	"testing",
}

type (
	Encryption struct {
		// Enabled determines whether encryption is enabled. Decryption will be attempted for encrypted
		// payloads regardless of this flag.
		Enabled bool `yaml:"enabled"`

		// Policy defines keys and rotation policies.
		Policy KeyPolicy `yaml:"policy"`
	}

	KeyPolicy struct {
		// URI is the vendor-specific URL identifying the KMS key used to encrypt DEKs.
		// URIs follow the gocloud.dev/secrets URL scheme (with the exception of the testing scheme):
		//
		//	GCP KMS:     gcpkms://projects/PROJECT/locations/LOCATION/keyRings/RING/cryptoKeys/KEY
		//	AWS KMS:     awskms:///arn:aws:kms:REGION:ACCOUNT:key/KEY-ID?region=REGION
		//	Azure Vault: azurekeyvault://VAULT.vault.azure.net/keys/KEY-NAME/KEY-VERSION
		//	Local/test:  testing://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4=
		URI string `yaml:"uri"`

		// RetiredURIs lists KMS keys that are no longer used to encrypt new DEKs but
		// are still needed to decrypt DEKs encrypted by previous keys (e.g. after a
		// provider migration). Each entry follows the same URI scheme rules as [URI].
		RetiredURIs []string `yaml:"retiredURIs"`

		// Duration is how long the DEK is valid before it must be rotated.
		Duration time.Duration `yaml:"duration"`

		// RenewBefore is how far before a DEK expires it should be proactively rotated.
		RenewBefore time.Duration `yaml:"renewBefore"`
	}
)

func (p *KeyPolicy) UnmarshalYAML(unmarshal func(any) error) error {
	type raw KeyPolicy
	var decoded raw
	if err := unmarshal(&decoded); err != nil {
		return err
	}

	*p = KeyPolicy(decoded)
	return p.validURIs()
}

func (p *KeyPolicy) validURIs() error {
	if err := validKeyURI(p.URI); err != nil {
		if strings.TrimSpace(p.URI) != "" {
			return err
		}
	}

	for _, uri := range p.RetiredURIs {
		if err := validKeyURI(uri); err != nil {
			return err
		}
	}

	return nil
}

func validKeyURI(uri string) error {
	u, err := url.Parse(uri)
	if err != nil {
		return fmt.Errorf("failed to parse key URI: %s, %w", uri, err)
	}

	if !slices.Contains(validKeySchemes, strings.ToLower(u.Scheme)) {
		return fmt.Errorf(
			"invalid key URI: %s, valid schemes: [%s]",
			uri,
			strings.Join(validKeySchemes, ","),
		)
	}

	return nil
}
