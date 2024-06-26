package rpc

import "crypto/tls"

type (
	// TLSConfigProvider serves as a common interface to read server and client configuration for TLS.
	TLSConfigProvider interface {
		GetRemoteClusterClientConfig(hostname string) (*tls.Config, error)
	}
)
