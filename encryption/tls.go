package encryption

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go.temporal.io/server/common/auth"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

type (
	ClientTLSConfig struct {
		CertificatePath string `json:"cert_path,omitempty"`
		KeyPath         string `json:"key_path,omitempty"`
		ServerName      string `json:"server_name,omitempty"`
		ServerCAPath    string `json:"server_ca_path,omitempty"`
	}

	HttpGetter interface {
		Get(url string) (resp *http.Response, err error)
	}

	TLSConfigProvider interface {
		GetClientTLSConfig(clientConfig ClientTLSConfig) (*tls.Config, error)
	}

	tlsConfigProvider struct {
		logger log.Logger
	}
)

func (c ClientTLSConfig) IsTlsEnabled() bool {
	if c.CertificatePath != "" && c.KeyPath != "" {
		// has valid config for client auth.
		return true
	}

	if c.ServerName != "" && c.ServerCAPath != "" {
		// has valid config for server auth.
		return true
	}

	return false
}

var netClient HttpGetter = &http.Client{
	Timeout: time.Second * 10,
}

func NewTLSConfigProfilder(
	logger log.Logger,
) TLSConfigProvider {
	return &tlsConfigProvider{
		logger: logger,
	}
}

func (t *tlsConfigProvider) GetClientTLSConfig(tlsConfig ClientTLSConfig) (*tls.Config, error) {
	certPath := tlsConfig.CertificatePath
	keyPath := tlsConfig.KeyPath
	caPath := tlsConfig.ServerCAPath
	serverName := tlsConfig.ServerName
	enableHostVerification := serverName != "" && caPath != ""

	var cert *tls.Certificate
	var caPool *x509.CertPool

	if caPath != "" {
		caCertPool, err := fetchCACert(caPath)
		if err != nil {
			t.logger.Fatal("Failed to load server CA certificate", tag.Error(err))
			return nil, err
		}
		caPool = caCertPool
	}

	if certPath != "" {
		myCert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			t.logger.Fatal("Failed to load client certificate", tag.Error(err))
			return nil, err
		}
		cert = &myCert
	}

	// If we are given arguments to verify either server or client, configure TLS
	if caPool != nil || cert != nil {
		tlsConfig := auth.NewTLSConfigForServer(serverName, enableHostVerification)
		if caPool != nil {
			tlsConfig.RootCAs = caPool
		}
		if cert != nil {
			tlsConfig.Certificates = []tls.Certificate{*cert}
		}

		return tlsConfig, nil
	}

	// If we are given a server name, set the TLS server name for DNS resolution
	if serverName != "" {
		tlsConfig := auth.NewTLSConfigForServer(serverName, enableHostVerification)
		return tlsConfig, nil
	}

	return nil, nil
}

func fetchCACert(pathOrUrl string) (caPool *x509.CertPool, err error) {
	caPool = x509.NewCertPool()
	var caBytes []byte

	if strings.HasPrefix(pathOrUrl, "http://") {
		return nil, errors.New("HTTP is not supported for CA cert URLs. Provide HTTPS URL")
	}

	if strings.HasPrefix(pathOrUrl, "https://") {
		resp, err := netClient.Get(pathOrUrl)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		caBytes, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
	} else {
		caBytes, err = os.ReadFile(pathOrUrl)
		if err != nil {
			return nil, err
		}
	}

	if !caPool.AppendCertsFromPEM(caBytes) {
		return nil, errors.New("unknown failure constructing cert pool for ca")
	}
	return caPool, nil
}
