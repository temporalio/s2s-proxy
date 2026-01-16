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
	// TLSConfig sets TLS options for the proxy's clients and servers
	TLSConfig struct {
		// CertificatePath is the path to the TLS cert that identifies this host
		CertificatePath string `yaml:"certificatePath"`
		// KeyPath is the path to the TLS key used to encrypt traffic
		KeyPath string `yaml:"keyPath"`
		// RemoteCAPath is the path to the TLS CA cert that is used to verify the remote host's certificate
		RemoteCAPath string `yaml:"remoteCAPath"`
		// CAServerName must match against the remote host's CA cert
		CAServerName string `yaml:"caServerName"`
		// If set to false, SkipCAVerification will skip the CA authentication step
		SkipCAVerification bool `yaml:"skipCAVerification"`
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

func GetServerTLSConfig(serverConfig TLSConfig, logger log.Logger) (tlsConfig *tls.Config, err error) {
	if !serverConfig.IsEnabled() {
		logger.Info("TLS disabled")
		return
	}

	tlsConfig = auth.NewEmptyTLSConfig()
	if !serverConfig.SkipCAVerification {
		tlsConfig.ClientAuth = tls.RequireAnyClientCert
		tlsConfig.ClientCAs, err = fetchCACert(serverConfig.RemoteCAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read CACert from %s: %w", serverConfig.RemoteCAPath, err)
		}
	} else {
		tlsConfig.ClientAuth = tls.NoClientCert
	}

	if serverConfig.CertificatePath != "" {
		serverCert, err := tls.LoadX509KeyPair(serverConfig.CertificatePath, serverConfig.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load cert-key pair (%s, %s): %w",
				serverConfig.CertificatePath, serverConfig.KeyPath, err)
		}
		tlsConfig.Certificates = []tls.Certificate{serverCert}
	}

	tlsConfig.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		logger.Info("Received TLS handshake",
			tag.Address(hello.Conn.RemoteAddr().String()), tag.ServerName(hello.ServerName))
		if len(tlsConfig.Certificates) > 0 {
			err := hello.SupportsCertificate(&tlsConfig.Certificates[0])
			if err != nil {
				logger.Warn("Client does not support our certificate! Dumping config",
					tag.Error(err), tag.NewStringTag("tlsConfig", fmt.Sprintf("%+v", tlsConfig)))
			}
		}
		return nil, nil
	}

	tlsConfig.GetClientCertificate = func(clientInfo *tls.CertificateRequestInfo) (*tls.Certificate, error) {
		logger.Info("Getting client cert")
		var allCertFailures error
		for _, cert := range tlsConfig.Certificates {
			certErr := clientInfo.SupportsCertificate(&cert)
			if certErr == nil {
				logger.Info("Handshake accepted")
				return &cert, nil
			}
			allCertFailures = errors.Join(allCertFailures, certErr)
		}
		logger.Warn("Could not match cert request. Check cert failures in error tag", tag.Error(allCertFailures))
		return nil, allCertFailures
	}

	tlsConfig.VerifyPeerCertificate = func(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			logger.Info("No client certificate provided, so no verification performed")
		} else {
			cert, _ := x509.ParseCertificate(rawCerts[0])
			logger.Info(fmt.Sprintf("Client certificate subject: %s", cert.Subject))
		}
		return nil
	}

	return
}

func GetClientTLSConfig(clientConfig TLSConfig) (tlsConfig *tls.Config, err error) {
	if !clientConfig.IsEnabled() {
		return
	}

	tlsConfig = auth.NewEmptyTLSConfig()
	tlsConfig.InsecureSkipVerify = clientConfig.SkipCAVerification
	if !clientConfig.SkipCAVerification {
		if clientConfig.CAServerName == "" || clientConfig.RemoteCAPath == "" {
			return nil, errors.New("CAServerName and RemoteCAPath must be set when SkipCAVerification is false")
		}
		tlsConfig.ServerName = clientConfig.CAServerName
	}

	if clientConfig.RemoteCAPath != "" {
		caCertPool, err := fetchCACert(clientConfig.RemoteCAPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA cert: %w", err)
		}
		tlsConfig.RootCAs = caCertPool
	}

	if clientConfig.CertificatePath != "" {
		myCert, err := tls.LoadX509KeyPair(clientConfig.CertificatePath, clientConfig.KeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load cert-key pair from path %s: %w", clientConfig.CertificatePath, err)
		}
		tlsConfig.Certificates = []tls.Certificate{myCert}
	}

	return
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
