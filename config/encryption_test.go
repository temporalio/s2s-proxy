package config_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/temporalio/s2s-proxy/config"
)

func TestKeyPolicyURIs(t *testing.T) {
	tests := []struct {
		name    string
		uri     string
		wantErr bool
	}{
		{name: "empty"},
		{name: "gcpkms", uri: "gcpkms://projects/p/locations/global/keyRings/r/cryptoKeys/k"},
		{name: "awskms", uri: "awskms:///arn:aws:kms:us-east-1:123456789012:key/abc?region=us-east-1"},
		{name: "azurekeyvault", uri: "azurekeyvault://my-vault.vault.azure.net/keys/my-key/v1"},
		{name: "testing", uri: "testing://smGbjm71Nxd1Ig5FS0wj9SlbzAIrnolCz9bQQ6uAhl4="},
		{name: "unknown scheme", uri: "hashivault://localhost/v1/transit/keys/my-key", wantErr: true},
		{name: "unparseable URI", uri: "://bad", wantErr: true},
	}

	for _, tt := range tests {
		str := "policy:\n  uri: " + tt.uri

		var enc config.Encryption
		err := yaml.Unmarshal([]byte(str), &enc)
		if tt.wantErr {
			require.Error(t, err, tt.name)
			continue
		}

		require.NoError(t, err, tt.name)
	}

	// Verify retired URIs are also validated
	for _, tt := range tests {
		tt.name += " retired"
		if tt.uri == "" {
			tt.wantErr = true
		}

		str := fmt.Sprintf("policy:\n  retiredURIs:\n  - \"%s\"", tt.uri)

		var enc config.Encryption
		err := yaml.Unmarshal([]byte(str), &enc)
		if tt.wantErr {
			require.Error(t, err, tt.name)
			continue
		}

		require.NoError(t, err, tt.name)
	}
}
