package encryption

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
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
	TLSConfig struct {
		CertificatePath  string `yaml:"certificatePath"`
		KeyPath          string `yaml:"keyPath"`
		RemoteCAPath     string `yaml:"remoteCAPath"`
		CAServerName     string `yaml:"caServerName"`
		ValidateClientCA bool   `yaml:"validateClientCA"`
	}

	HttpGetter interface {
		Get(url string) (resp *http.Response, err error)
	}
)

func (t TLSConfig) IsEnabled() bool {
	return (t.CertificatePath != "" && t.KeyPath != "") || (t.RemoteCAPath != "" && t.CAServerName != "")
}

var netClient HttpGetter = &http.Client{
	Timeout: time.Second * 10,
}

func GetServerTLSConfig(serverConfig TLSConfig, logger log.Logger) (*tls.Config, error) {
	certPath := serverConfig.CertificatePath
	keyPath := serverConfig.KeyPath
	clientCAPath := serverConfig.RemoteCAPath

	if !serverConfig.IsEnabled() {
		return nil, nil
	}

	var serverCert *tls.Certificate
	var clientCAPool *x509.CertPool

	clientAuthType := tls.NoClientCert
	if serverConfig.ValidateClientCA {
		clientAuthType = tls.RequireAndVerifyClientCert
		caCertPool, err := fetchCACert(clientCAPath)
		if err != nil {
			return nil, err
		}
		clientCAPool = caCertPool
	}

	if certPath != "" {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, err
		}
		serverCert = &cert
	}

	c := auth.NewEmptyTLSConfig()
	c.ClientAuth = clientAuthType
	c.Certificates = []tls.Certificate{*serverCert}
	c.ClientCAs = clientCAPool
	c.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		logger.Info("Received TLS handshake", tag.Address(hello.Conn.RemoteAddr().String()))
		return nil, nil
	}

	c.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			logger.Info("No client certificate provided")
		} else {
			cert, _ := x509.ParseCertificate(rawCerts[0])
			logger.Info(fmt.Sprintf("Client certificate subject: %s", cert.Subject))
		}
		return nil
	}

	return c, nil
}

func GetClientTLSConfig(clientConfig TLSConfig) (*tls.Config, error) {
	certPath := clientConfig.CertificatePath
	keyPath := clientConfig.KeyPath
	caPath := clientConfig.RemoteCAPath
	serverName := clientConfig.CAServerName

	if !clientConfig.IsEnabled() {
		return nil, nil
	}

	var cert *tls.Certificate
	var caPool *x509.CertPool

	if caPath != "" {
		caCertPool, err := fetchCACert(caPath)
		if err != nil {
			return nil, err
		}
		caPool = caCertPool
	}

	if certPath != "" {
		myCert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
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
		defer func() { _ = resp.Body.Close() }()
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
