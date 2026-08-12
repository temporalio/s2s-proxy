package proxy

import (
	"context"
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/logging"
)

const peerSAN = "s2s-proxy-peers"

// peerServerTLSConfig exists because encryption.GetServerTLSConfig never checks the client chain.
// That function sets RequireAnyClientCert and replaces VerifyPeerCertificate with one that logs the subject and returns nil.
// This asserts the difference: a certificate from another CA is refused.
func TestPeerListenerRejectsACertificateFromAnotherCA(t *testing.T) {
	ours := newTestCA(t, "ours")
	theirs := newTestCA(t, "theirs")

	serverCert, serverKey := ours.issue(t, "server", peerSAN)
	peerTLS := encryption.TLSConfig{
		CertificatePath: serverCert,
		KeyPath:         serverKey,
		RemoteCAPath:    ours.certPath,
		CAServerName:    peerSAN,
	}

	built, err := peerServerTLSConfig(peerTLS)
	require.NoError(t, err)
	require.Equal(t, tls.RequireAndVerifyClientCert, built.ClientAuth,
		"anything weaker accepts a certificate without checking who signed it")
	require.Nil(t, built.VerifyPeerCertificate,
		"an override here would replace Go's chain verification with whatever it returns")

	address := startPeerTLSServer(t, peerTLS)

	t.Run("a peer signed by our CA is accepted", func(t *testing.T) {
		cert, key := ours.issue(t, "sibling", peerSAN)
		require.NoError(t, callPeer(t, address, ours.certPath, cert, key))
	})

	t.Run("a peer signed by another CA is refused", func(t *testing.T) {
		cert, key := theirs.issue(t, "stranger", peerSAN)
		// The stranger still trusts this server.
		// The failure is the server rejecting the client rather than the client rejecting the server.
		err := callPeer(t, address, ours.certPath, cert, key)
		require.Error(t, err)
	})

	t.Run("a peer presenting no certificate is refused", func(t *testing.T) {
		require.Error(t, callPeer(t, address, ours.certPath, "", ""))
	})
}

// startPeerTLSServer brings up a proxy whose peer listener uses tlsCfg, through the real config path.
// peerServerTLSConfig is therefore the code under test rather than a hand-built tls.Config.
func startPeerTLSServer(t *testing.T, tlsCfg encryption.TLSConfig) string {
	t.Helper()
	loggers := logging.NewLoggerProvider(log.NewTestLogger(), config.NewMockConfigProvider(config.S2SProxyConfig{}))
	a := getDynamicPlccAddresses(t)
	address := dynamicAddress(t)

	proxy, err := NewProxy(config.NewMockConfigProvider(config.S2SProxyConfig{
		ProxyAdmin: config.ProxyAdminConfig{
			Peer: &config.ProxyAdminPeerConfig{ListenAddress: address, TLS: &tlsCfg},
		},
		ClusterConnections: []config.ClusterConnConfig{
			makeMuxClusterConfig("conn", config.ConnTypeMuxServer, localFVI, remoteFVI,
				a.localProxyOutbound, a.localTemporalAddr, a.localProxyOutbound, a.localProxyInbound),
		},
	}), loggers, Identity{Version: "v-test", MemberID: "pod-a"})
	require.NoError(t, err)
	require.NoError(t, proxy.Start())
	t.Cleanup(proxy.Stop)
	require.Len(t, proxy.adminServers, 1, "the peer listener must be up for this to test anything")
	return address
}

// callPeer dials the peer listener and makes one call.
// An empty certPath sends no client certificate at all.
func callPeer(t *testing.T, address, caPath, certPath, keyPath string) error {
	t.Helper()
	pool, err := encryption.LoadCACert(caPath)
	require.NoError(t, err)

	clientTLS := &tls.Config{RootCAs: pool, ServerName: peerSAN, MinVersion: tls.VersionTLS12}
	if certPath != "" {
		pair, err := tls.LoadX509KeyPair(certPath, keyPath)
		require.NoError(t, err)
		clientTLS.Certificates = []tls.Certificate{pair}
	}

	conn, err := grpc.NewClient(address, grpc.WithTransportCredentials(credentials.NewTLS(clientTLS)))
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = proxyadminv1.NewProxyAdminServiceClient(conn).
		DescribeClusterConnections(ctx, &proxyadminv1.DescribeClusterConnectionsRequest{})
	return err
}
