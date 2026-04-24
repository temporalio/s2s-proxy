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
		tlsConfig TLSConfig
		isEnabled bool
	}{
		{
			name: "client-auth-and-server-auth-enabled",
			tlsConfig: TLSConfig{
				CertificatePath: "path",
				KeyPath:         "path",
				CAServerName:    "server",
				RemoteCAPath:    "path",
			},
			isEnabled: true,
		},
		{
			name: "client-auth-enabled",
			tlsConfig: TLSConfig{
				CertificatePath: "path",
				KeyPath:         "path",
			},
			isEnabled: true,
		},
		{
			name: "server-auth-enabled",
			tlsConfig: TLSConfig{
				CAServerName: "server",
				RemoteCAPath: "path",
			},
			isEnabled: true,
		},
		{
			name:      "auth-disabled",
			tlsConfig: TLSConfig{},
			isEnabled: false,
		},
		{
			name: "partialauth-disabled",
			tlsConfig: TLSConfig{
				CertificatePath: "path",
				CAServerName:    "server",
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
