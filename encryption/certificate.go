package encryption

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
)

var errNoCACertificates = errors.New("no usable CA certificates found in bundle")

func isCACert(cert *x509.Certificate) bool {
	return cert.IsCA
}

func certificatesFromPEM(pemBytes []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemBytes
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}
		certs = append(certs, cert)
	}
	return certs, nil
}

func validateHasCA(certs []*x509.Certificate, source string) error {
	if !slices.ContainsFunc(certs, isCACert) {
		return fmt.Errorf("ca file %q: %w", source, errNoCACertificates)
	}
	return nil
}
