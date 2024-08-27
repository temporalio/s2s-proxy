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
	ServerTLSConfig struct {
		CertificatePath   string `yaml:"certificatePath"`
		KeyPath           string `yaml:"keyPath"`
		ClientCAPath      string `yaml:"clientCAPath"`
		RequireClientAuth bool   `yaml:"requireClientAuth"`
	}

	ClientTLSConfig struct {
		CertificatePath string `yaml:"certificatePath"`
		KeyPath         string `yaml:"keyPath"`
		ServerName      string `yaml:"serverName"`
		ServerCAPath    string `yaml:"serverCAPath"`
	}

	HttpGetter interface {
		Get(url string) (resp *http.Response, err error)
	}

	TLSConfigProvider interface {
		GetServerTLSConfig(serverConfig ServerTLSConfig) (*tls.Config, error)
		GetClientTLSConfig(clientConfig ClientTLSConfig) (*tls.Config, error)
	}

	tlsConfigProvider struct {
		logger log.Logger
	}
)

func (c ClientTLSConfig) IsEnabled() bool {
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

func (t *tlsConfigProvider) GetServerTLSConfig(serverConfig ServerTLSConfig) (*tls.Config, error) {
	return nil, nil
}

func (t *tlsConfigProvider) GetClientTLSConfig(clientConfig ClientTLSConfig) (*tls.Config, error) {
	certPath := clientConfig.CertificatePath
	keyPath := clientConfig.KeyPath
	caPath := clientConfig.ServerCAPath
	serverName := clientConfig.ServerName

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
	if caPool != nil || cert != nil || serverName != "" {
		enableHostVerification := serverName != "" && caPath != ""
		tlsConfig := auth.NewTLSConfigForServer(serverName, enableHostVerification)
		if caPool != nil {
			tlsConfig.RootCAs = caPool
		}
		if cert != nil {
			tlsConfig.Certificates = []tls.Certificate{*cert}
		}

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
