package proxy

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testCA is a certificate authority and the leaves it signs, written to a temp directory.
//
// The committed fixtures under develop/certificates are two self-signed leaves that each trust the other directly.
// They cannot express "signed by a different CA".
// That distinction is the entire claim of peerServerTLSConfig.
//
// It lives here because proxy is its only consumer.
type testCA struct {
	dir      string
	certPath string
	keyPath  string
	cert     *x509.Certificate
	key      *rsa.PrivateKey
}

func newTestCA(t *testing.T, commonName string) *testCA {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)

	ca := &testCA{dir: t.TempDir(), cert: cert, key: key}
	ca.certPath = writePEM(t, ca.dir, commonName+"-ca.pem", "CERTIFICATE", der)
	ca.keyPath = writePEM(t, ca.dir, commonName+"-ca.key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
	return ca
}

// issue returns paths to a leaf signed by this CA, with dnsName as its only SAN.
//
// Peers are dialed by IP.
// The SAN has to be one name every pod's certificate shares rather than a per-pod name.
// That is the property caServerName selects on.
func (c *testCA) issue(t *testing.T, name, dnsName string) (certPath, keyPath string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{dnsName},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, c.cert, &key.PublicKey, c.key)
	require.NoError(t, err)

	return writePEM(t, c.dir, name+".pem", "CERTIFICATE", der),
		writePEM(t, c.dir, name+".key", "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key))
}

func writePEM(t *testing.T, dir, name, blockType string, der []byte) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600))
	return path
}
