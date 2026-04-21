package encryption

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/server/common/log"
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

func (s *tlsTestSuite) Test_ValidateCAPoolPEM() {
	t := s.T()

	cases := []struct {
		name    string
		pem     []byte
		wantErr string
	}{
		{"valid CA is accepted", caPEM(t, "root"), ""},
		{"leaf cert is rejected", leafPEM(t, "server"), `"server" is not a CA`},
		{"cert with no BasicConstraints is rejected", unconstrainedPEM(t, "noBasicConstraints"), `"noBasicConstraints" is not a CA`},
		{"nonsensical PEM is rejected", []byte("not a cert"), "no CA certificates found"},
		{"empty input is rejected", nil, "no CA certificates found"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			err := validateCAPoolPEM(tc.pem, "ca.pem", log.NewTestLogger())
			if tc.wantErr == "" {
				s.Require().NoError(err)
				return
			}
			s.Require().ErrorContains(err, tc.wantErr)
		})
	}
}

func createCert(t *testing.T, tmpl x509.Certificate) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func caPEM(t *testing.T, cn string) []byte {
	return createCert(t, x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	})
}

func leafPEM(t *testing.T, cn string) []byte {
	return createCert(t, x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: cn},
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  false,
		BasicConstraintsValid: true,
	})
}

func unconstrainedPEM(t *testing.T, cn string) []byte {
	return createCert(t, x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: cn},
		NotAfter:     time.Now().Add(time.Hour),
	})
}
