package preflight

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
)

func TestCheckProxyWithOptions_HealthyCustomerProxy(t *testing.T) {
	now := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	certificatePath, keyPath, _ := writeCertificateFiles(t, now.Add(-time.Hour), now.Add(90*24*time.Hour))
	cfg := healthyProxyConfig(certificatePath, keyPath)
	configPath := writeProxyConfig(t, cfg)
	info, err := os.Stat(configPath)
	require.NoError(t, err)

	var checkedListeners []string
	report := CheckProxyWithOptions(configPath, "v-test", ProxyCheckOptions{
		Now:      func() time.Time { return now },
		Hostname: func() (string, error) { return "proxy-pod-0", nil },
		ProcessStartTime: func() (time.Time, error) {
			return info.ModTime().Add(time.Hour), nil
		},
		CheckListener: func(address string) error {
			checkedListeners = append(checkedListeners, address)
			return nil
		},
	})

	assert.False(t, report.HasFailures())
	assert.False(t, report.HasUnknowns())
	assert.NotEqual(t, "unknown", report.ConfigSHA256)
	assert.Len(t, report.ConfigSHA256, 64)
	assert.Equal(t, "proxy-pod-0", report.Pod)
	assert.Equal(t, []string{"0.0.0.0:9233", "0.0.0.0:8234", "0.0.0.0:9090", "localhost:6060"}, checkedListeners)
	assert.Equal(t, StatusInfo, resultNamed(t, report, "effective config").Status)
	assert.Contains(t, strings.Join(resultNamed(t, report, "effective config").Details, "\n"), "muxCount=10")
	for _, result := range report.Results {
		assert.NotEqual(t, StatusWarn, result.Status, result.Name)
	}
}

func TestCheckProxyWithOptions_ConfigReadAndFormatFailures(t *testing.T) {
	opts := ProxyCheckOptions{
		Hostname:         func() (string, error) { return "proxy-pod-0", nil },
		ProcessStartTime: func() (time.Time, error) { return time.Now(), nil },
		CheckListener:    func(string) error { return nil },
	}

	t.Run("missing file", func(t *testing.T) {
		report := CheckProxyWithOptions(filepath.Join(t.TempDir(), "missing.yaml"), "v-test", opts)
		require.Len(t, report.Results, 1)
		assert.Equal(t, StatusFail, report.Results[0].Status)
		assert.Equal(t, "unknown", report.ConfigSHA256)
	})

	t.Run("malformed yaml", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("clusterConnections: ["), 0o600))
		report := CheckProxyWithOptions(path, "v-test", opts)
		require.Len(t, report.Results, 1)
		assert.Equal(t, "config file does not parse", report.Results[0].Name)
	})

	t.Run("unknown field", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("clusterConnections: []\nlegacyField: true\n"), 0o600))
		report := CheckProxyWithOptions(path, "v-test", opts)
		require.Len(t, report.Results, 2)
		assert.Equal(t, "config format is not supported", report.Results[1].Name)
	})

	t.Run("multiple yaml documents", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.yaml")
		require.NoError(t, os.WriteFile(path, []byte("clusterConnections: []\n---\nclusterConnections: []\n"), 0o600))
		report := CheckProxyWithOptions(path, "v-test", opts)
		require.Len(t, report.Results, 1)
		assert.Equal(t, "config file does not parse", report.Results[0].Name)
	})
}

func TestCheckConfigStructure(t *testing.T) {
	cfg := healthyProxyConfig("cert.pem", "key.pem")
	assert.Equal(t, StatusPass, checkConfigStructure(cfg).Status)

	t.Run("customer side shape", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].Remote.ConnectionType = config.ConnTypeMuxServer
		result := checkConfigStructure(cfg)
		assert.Equal(t, StatusFail, result.Status)
		assert.Contains(t, strings.Join(result.Details, "\n"), "must be mux-client")
	})

	t.Run("placeholder remote address", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].Remote.MuxAddressInfo.ConnectionString = "remote_proxy_service:8233"
		result := checkConfigStructure(cfg)
		assert.Equal(t, StatusFail, result.Status)
		assert.Contains(t, strings.Join(result.Details, "\n"), "template placeholder")
	})

	t.Run("duplicate listener", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.Metrics.Prometheus.ListenAddress = ":9233"
		result := checkConfigStructure(cfg)
		assert.Equal(t, StatusFail, result.Status)
		assert.Contains(t, strings.Join(result.Details, "\n"), "same listener address")
	})

	t.Run("invalid port", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].Local.TcpClient.ConnectionString = "temporal.example:0"
		assert.Equal(t, StatusFail, checkConfigStructure(cfg).Status)
	})
}

func TestCheckTLSConfiguration(t *testing.T) {
	t.Run("remote tls is required", func(t *testing.T) {
		cfg := healthyProxyConfig("", "")
		assert.Equal(t, StatusFail, checkTLSConfiguration(cfg).Status)
	})

	t.Run("remote ca alone does not enable tls", func(t *testing.T) {
		cfg := healthyProxyConfig("", "")
		cfg.ClusterConnections[0].Remote.MuxAddressInfo.TLSConfig.RemoteCAPath = "ca.pem"
		result := checkTLSConfiguration(cfg)
		assert.Equal(t, StatusFail, result.Status)
		assert.Contains(t, strings.Join(result.Details, "\n"), "requires a certificate and key")
	})

	t.Run("empty local tls with skip verification is disabled", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].Local.TcpClient.TLSConfig.SkipCAVerification = true
		cfg.ClusterConnections[0].Local.TcpServer.TLSConfig.SkipCAVerification = true
		assert.Equal(t, StatusPass, checkTLSConfiguration(cfg).Status)
	})

	t.Run("local client may use server-only tls", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].Local.TcpClient.TLSConfig.CAServerName = "temporal.example"
		assert.Equal(t, StatusPass, checkTLSConfiguration(cfg).Status)
	})
}

func TestACLAndNamespaceChecks(t *testing.T) {
	t.Run("empty acl is unrestricted", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].ACLPolicy = nil
		assert.Equal(t, StatusSkip, checkAdminPermissions(cfg).Status)
		assert.Equal(t, StatusSkip, checkTemporalSystem(cfg).Status)
	})

	t.Run("partial admin acl fails", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].ACLPolicy.AllowedMethods.AdminService = []string{"DescribeCluster"}
		result := checkAdminPermissions(cfg)
		assert.Equal(t, StatusFail, result.Status)
		assert.Contains(t, strings.Join(result.Details, "\n"), "SyncWorkflowState")
	})

	t.Run("temporal system is required when namespaces are restricted", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].ACLPolicy.AllowedNamespaces = []string{"orders"}
		assert.Equal(t, StatusFail, checkTemporalSystem(cfg).Status)
	})

	t.Run("acl uses local namespace names", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].ACLPolicy.AllowedNamespaces = []string{temporalSystem, "orders.cloud-account"}
		assert.Equal(t, StatusFail, checkAllowedNamespaceSide(cfg).Status)
	})

	t.Run("allowed namespace without mapping is a concern", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].NamespaceTranslation.Mappings = nil
		result := checkNamespaceMappings(cfg)
		assert.Equal(t, StatusWarn, result.Status)
		assert.Contains(t, strings.Join(result.Details, "\n"), "valid only when both sides use the same name")
	})

	t.Run("template namespace fails", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].ACLPolicy.AllowedNamespaces = []string{temporalSystem, "myNamespace"}
		cfg.ClusterConnections[0].NamespaceTranslation.Mappings[0] = config.StringMapping{Local: "myNamespace", Remote: "myNamespace.accountid"}
		assert.Equal(t, StatusFail, checkNamespaceMappings(cfg).Status)
	})
}

func TestReplicationEndpointIsConcernNotCustomerSpecificVerdict(t *testing.T) {
	cfg := healthyProxyConfig("cert.pem", "key.pem")
	cfg.ClusterConnections[0].ReplicationEndpoint = "127.0.0.1:433"
	result := checkReplicationEndpoints(cfg)
	assert.Equal(t, StatusWarn, result.Status)
	assert.Contains(t, strings.Join(result.Details, "\n"), "internal routing translates ports")
}

func TestIdentityAndNumericChecks(t *testing.T) {
	t.Run("duplicate names", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections = append(cfg.ClusterConnections, cfg.ClusterConnections[0])
		assert.Equal(t, StatusFail, checkConnectionNames(cfg).Status)
	})

	t.Run("inactive shard counts warn", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].ShardCountConfig = config.ShardCountConfig{LocalShardCount: 4, RemoteShardCount: 8}
		assert.Equal(t, StatusWarn, checkNumericSettings(cfg).Status)
	})

	t.Run("lcm overflow fails", func(t *testing.T) {
		cfg := healthyProxyConfig("cert.pem", "key.pem")
		cfg.ClusterConnections[0].ShardCountConfig = config.ShardCountConfig{
			Mode:             config.ShardCountLCM,
			LocalShardCount:  2_147_483_647,
			RemoteShardCount: 2_147_483_646,
		}
		assert.Equal(t, StatusFail, checkNumericSettings(cfg).Status)
	})
}

func TestCertificateFiles(t *testing.T) {
	now := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)

	t.Run("valid pair loads", func(t *testing.T) {
		certificatePath, keyPath, caPath := writeCertificateFiles(t, now.Add(-time.Hour), now.Add(90*24*time.Hour))
		cfg := healthyProxyConfig(certificatePath, keyPath)
		cfg.ClusterConnections[0].Remote.MuxAddressInfo.TLSConfig.RemoteCAPath = caPath
		assert.Equal(t, StatusPass, checkCertificateFiles(cfg, now).Status)
	})

	t.Run("missing key fails", func(t *testing.T) {
		certificatePath, _, _ := writeCertificateFiles(t, now.Add(-time.Hour), now.Add(90*24*time.Hour))
		cfg := healthyProxyConfig(certificatePath, filepath.Join(t.TempDir(), "missing.key"))
		assert.Equal(t, StatusFail, checkCertificateFiles(cfg, now).Status)
	})

	t.Run("near expiry warns", func(t *testing.T) {
		certificatePath, keyPath, _ := writeCertificateFiles(t, now.Add(-time.Hour), now.Add(7*24*time.Hour))
		cfg := healthyProxyConfig(certificatePath, keyPath)
		assert.Equal(t, StatusWarn, checkCertificateFiles(cfg, now).Status)
	})

	t.Run("https ca is not fetched", func(t *testing.T) {
		certificatePath, keyPath, _ := writeCertificateFiles(t, now.Add(-time.Hour), now.Add(90*24*time.Hour))
		cfg := healthyProxyConfig(certificatePath, keyPath)
		cfg.ClusterConnections[0].Remote.MuxAddressInfo.TLSConfig.RemoteCAPath = "https://example.test/ca.pem"
		assert.Equal(t, StatusWarn, checkCertificateFiles(cfg, now).Status)
	})
}

func TestRuntimeChecks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("clusterConnections: []\n"), 0o600))
	info, err := os.Stat(path)
	require.NoError(t, err)

	assert.Equal(t, StatusPass, checkConfigFreshness(path, func() (time.Time, error) {
		return info.ModTime().Add(time.Hour), nil
	}).Status)
	assert.Equal(t, StatusWarn, checkConfigFreshness(path, func() (time.Time, error) {
		return info.ModTime().Add(-time.Hour), nil
	}).Status)
	assert.Equal(t, StatusUnknown, checkConfigFreshness(path, func() (time.Time, error) {
		return time.Time{}, errors.New("not in proxy pod")
	}).Status)

	cfg := healthyProxyConfig("cert.pem", "key.pem")
	result := checkLocalListeners(cfg, func() (time.Time, error) { return time.Now(), nil }, func(address string) error {
		if address == "0.0.0.0:9233" {
			return errors.New("connection refused")
		}
		return nil
	})
	assert.Equal(t, StatusFail, result.Status)
	assert.Contains(t, strings.Join(result.Details, "\n"), "connection refused")
}

func healthyProxyConfig(certificatePath string, keyPath string) config.S2SProxyConfig {
	methods := slices.Clone(requiredAdminMethods)
	return config.S2SProxyConfig{
		ClusterConnections: []config.ClusterConnConfig{
			{
				Name: "orders-migration",
				Local: config.ClusterDefinition{
					ConnectionType: config.ConnTypeTCP,
					TcpClient:      config.TCPTLSInfo{ConnectionString: "temporal-frontend.temporal.svc.cluster.local:7233"},
					TcpServer:      config.TCPTLSInfo{ConnectionString: "0.0.0.0:9233"},
				},
				Remote: config.ClusterDefinition{
					ConnectionType: config.ConnTypeMuxClient,
					MuxAddressInfo: config.TCPTLSInfo{
						ConnectionString: "cloud-proxy.example.tmprl.cloud:8233",
						TLSConfig: encryption.TLSConfig{
							CertificatePath:    certificatePath,
							KeyPath:            keyPath,
							SkipCAVerification: true,
						},
					},
				},
				ReplicationEndpoint: "s2s-proxy.temporal.svc.cluster.local:9233",
				FVITranslation:      config.IntMapping{Local: 100, Remote: 1_000_000},
				ACLPolicy: &config.ACLPolicy{
					AllowedMethods:    config.AllowedMethods{AdminService: methods},
					AllowedNamespaces: []string{temporalSystem, "orders"},
				},
				NamespaceTranslation: config.StringTranslator{Mappings: []config.StringMapping{
					{Local: "orders", Remote: "orders.cloud-account"},
				}},
				RemoteClusterHealthCheck: config.HealthCheckConfig{Protocol: config.HTTP, ListenAddress: "0.0.0.0:8234"},
				ShardCountConfig: config.ShardCountConfig{
					Mode:             config.ShardCountLCM,
					LocalShardCount:  4,
					RemoteShardCount: 8,
				},
			},
		},
		Metrics:         &config.MetricsConfig{Prometheus: config.PrometheusConfig{ListenAddress: "0.0.0.0:9090"}},
		ProfilingConfig: &config.ProfilingConfig{PProfHTTPAddress: "localhost:6060"},
	}
}

func writeProxyConfig(t *testing.T, cfg config.S2SProxyConfig) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, config.WriteConfig(cfg, path))
	return path
}

func writeCertificateFiles(t *testing.T, notBefore time.Time, notAfter time.Time) (string, string, string) {
	t.Helper()
	directory := t.TempDir()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             notBefore.Add(-time.Hour),
		NotAfter:              notAfter.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	require.NoError(t, err)

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test-client"},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caTemplate, &leafKey.PublicKey, caKey)
	require.NoError(t, err)
	leafKeyDER, err := x509.MarshalECPrivateKey(leafKey)
	require.NoError(t, err)

	certificatePath := filepath.Join(directory, "client.pem")
	keyPath := filepath.Join(directory, "client.key")
	caPath := filepath.Join(directory, "ca.pem")
	require.NoError(t, os.WriteFile(certificatePath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER}), 0o600))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: leafKeyDER}), 0o600))
	require.NoError(t, os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o600))
	return certificatePath, keyPath, caPath
}

func resultNamed(t *testing.T, report Report, name string) Result {
	t.Helper()
	for _, result := range report.Results {
		if result.Name == name {
			return result
		}
	}
	t.Fatalf("result %q not found", name)
	return Result{}
}
