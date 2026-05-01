package encryption

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func (s *tlsTestSuite) Test_validateHasCA() {
	t := s.T()

	cases := []struct {
		name    string
		pem     []byte
		wantErr string
	}{
		{"valid CA is accepted", caPEM(t, "root"), ""},
		{"leaf-only bundle is rejected", leafPEM(t, "server"), "no usable CA certificates found"},
		{"unconstrained-only bundle is rejected", unconstrainedPEM(t, "noBasicConstraints"), "no usable CA certificates found"},
		{"mixed bundle with at least one CA is accepted", append(leafPEM(t, "server"), caPEM(t, "root")...), ""},
		{"nonsensical PEM is rejected", []byte("not a cert"), "no usable CA certificates found"},
		{"empty input is rejected", nil, "no usable CA certificates found"},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			certs, err := certificatesFromPEM(tc.pem)
			s.Require().NoError(err)
			err = validateHasCA(certs, "ca.pem")
			if tc.wantErr == "" {
				s.Require().NoError(err)
				return
			}
			s.Require().ErrorContains(err, tc.wantErr)
		})
	}
}

func (s *tlsTestSuite) Test_validateHasCA_WrapsSentinel() {
	certs, err := certificatesFromPEM(leafPEM(s.T(), "server"))
	s.Require().NoError(err)
	err = validateHasCA(certs, "ca.pem")
	s.Require().Error(err)
	s.Require().ErrorIs(err, errNoCACertificates)
}

func (s *tlsTestSuite) Test_certificatesFromPEM() {
	t := s.T()

	privateKeyBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: []byte("not a real key, but pem.Decode only cares about the wrapper"),
	})
	garbageCertBlock := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: []byte("not valid DER"),
	})

	cases := []struct {
		name      string
		pem       []byte
		wantCerts int
		wantErr   bool
	}{
		{
			name:      "single CA parses cleanly",
			pem:       caPEM(t, "root"),
			wantCerts: 1,
		},
		{
			name:      "PRIVATE KEY block is skipped alongside a CA",
			pem:       append(caPEM(t, "root"), privateKeyBlock...),
			wantCerts: 1,
		},
		{
			name:    "unparseable CERTIFICATE DER surfaces an error",
			pem:     garbageCertBlock,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			certs, err := certificatesFromPEM(tc.pem)
			if tc.wantErr {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)
			s.Require().Len(certs, tc.wantCerts)
		})
	}
}

func (s *tlsTestSuite) Test_isCACert() {
	t := s.T()

	parseFirst := func(pemBytes []byte) *x509.Certificate {
		certs, err := certificatesFromPEM(pemBytes)
		s.Require().NoError(err)
		s.Require().NotEmpty(certs)
		return certs[0]
	}

	s.True(isCACert(parseFirst(caPEM(t, "root"))))
	s.False(isCACert(parseFirst(leafPEM(t, "server"))))
	s.False(isCACert(parseFirst(unconstrainedPEM(t, "legacy"))))
}

func (s *tlsTestSuite) Test_fetchCACert() {
	t := s.T()

	caBytes := caPEM(t, "root")
	leafBytes := leafPEM(t, "server")
	mixed := append(append([]byte{}, leafBytes...), caBytes...)

	cases := []struct {
		name         string
		bundle       []byte
		wantErrIs    error
		wantPoolFrom []byte
	}{
		{
			name:      "rejects leaf-only bundle",
			bundle:    leafBytes,
			wantErrIs: errNoCACertificates,
		},
		{
			name:         "accepts CA-only bundle",
			bundle:       caBytes,
			wantPoolFrom: caBytes,
		},
		{
			name:         "accepts mixed bundle (pool contains both leaf and CA)",
			bundle:       mixed,
			wantPoolFrom: mixed,
		},
	}
	for _, tc := range cases {
		s.Run(tc.name, func() {
			path := filepath.Join(t.TempDir(), "ca.pem")
			s.Require().NoError(os.WriteFile(path, tc.bundle, 0600))

			pool, err := fetchCACert(path)
			if tc.wantErrIs != nil {
				s.Require().Error(err)
				s.Require().ErrorIs(err, tc.wantErrIs)
				return
			}
			s.Require().NoError(err)

			expected := x509.NewCertPool()
			s.Require().True(expected.AppendCertsFromPEM(tc.wantPoolFrom))
			s.True(pool.Equal(expected))
		})
	}
}

func (s *tlsTestSuite) Test_fetchCACert_MissingFile() {
	_, err := fetchCACert(filepath.Join(s.T().TempDir(), "does-not-exist.pem"))
	s.Require().Error(err)
	s.Require().True(errors.Is(err, os.ErrNotExist))
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
