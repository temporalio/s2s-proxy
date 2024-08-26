package encryption

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type (
	tlsTestSuite struct {
		suite.Suite
	}
)

func TestTLSTestSuite(t *testing.T) {
	suite.Run(t, new(tlsTestSuite))
}

func (s *tlsTestSuite) SetupTest() {
}

func (s *tlsTestSuite) AfterTest(suiteName, testName string) {
}

func (s *tlsTestSuite) Test_ClientTLSConfig() {
	tests := []struct {
		name      string
		tlsConfig ClientTLSConfig
		isEnabled bool
	}{
		{
			name: "client-auth-and-server-auth-enabled",
			tlsConfig: ClientTLSConfig{
				CertificatePath: "path",
				KeyPath:         "path",
				ServerName:      "server",
				ServerCAPath:    "path",
			},
			isEnabled: true,
		},
		{
			name: "client-auth-enabled",
			tlsConfig: ClientTLSConfig{
				CertificatePath: "path",
				KeyPath:         "path",
			},
			isEnabled: true,
		},
		{
			name: "server-auth-enabled",
			tlsConfig: ClientTLSConfig{
				ServerName:   "server",
				ServerCAPath: "path",
			},
			isEnabled: true,
		},
		{
			name:      "auth-disabled",
			tlsConfig: ClientTLSConfig{},
			isEnabled: false,
		},
		{
			name: "partialauth-disabled",
			tlsConfig: ClientTLSConfig{
				CertificatePath: "path",
				ServerName:      "server",
			},
			isEnabled: false,
		},
	}

	for _, ts := range tests {
		s.Run(
			ts.name,
			func() {
				s.Equal(ts.isEnabled, ts.tlsConfig.IsEnabled())
			},
		)
	}
}
