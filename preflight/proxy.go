package preflight

import (
	"bytes"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/procfs"
	"gopkg.in/yaml.v3"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

const (
	defaultListenerTimeout = 500 * time.Millisecond
	certificateWarningAge  = 30 * 24 * time.Hour
	temporalSystem         = "temporal-system"
)

var requiredAdminMethods = []string{
	"AddOrUpdateRemoteCluster",
	"RemoveRemoteCluster",
	"DescribeCluster",
	"DescribeMutableState",
	"GetNamespaceReplicationMessages",
	"GetWorkflowExecutionRawHistoryV2",
	"ListClusters",
	"StreamWorkflowReplicationMessages",
	"ReapplyEvents",
	"GetNamespace",
	"SyncWorkflowState",
}

type ProxyCheckOptions struct {
	Now              func() time.Time
	Hostname         func() (string, error)
	ProcessStartTime func() (time.Time, error)
	CheckListener    func(string) error
}

func CheckProxy(configPath string, version string) Report {
	return CheckProxyWithOptions(configPath, version, ProxyCheckOptions{})
}

func CheckProxyWithOptions(configPath string, version string, opts ProxyCheckOptions) Report {
	opts = defaultProxyCheckOptions(opts)
	pod, err := opts.Hostname()
	if err != nil || pod == "" {
		pod = "unknown"
	}
	report := Report{
		ConfigPath:   configPath,
		ConfigSHA256: "unknown",
		Version:      version,
		Pod:          pod,
		Scope:        GenericProxyScope,
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		report.Results = append(report.Results, Result{
			Status: StatusFail,
			Name:   "config file cannot be read",
			Details: []string{
				err.Error(),
			},
		})
		return report
	}
	report.ConfigSHA256 = fmt.Sprintf("%x", sha256.Sum256(data))

	if err := parseSingleYAMLDocument(data); err != nil {
		report.Results = append(report.Results, Result{
			Status:  StatusFail,
			Name:    "config file does not parse",
			Details: []string{err.Error()},
		})
		return report
	}
	report.Results = append(report.Results, Result{Status: StatusPass, Name: "config file parses"})

	cfg, err := config.LoadConfig[config.S2SProxyConfig](configPath)
	if err != nil {
		report.Results = append(report.Results, Result{
			Status:  StatusFail,
			Name:    "config format is not supported",
			Details: splitErrorLines(err),
		})
		return report
	}
	report.Results = append(report.Results, checkConfigStructure(cfg))
	report.Results = append(report.Results,
		checkTLSConfiguration(cfg),
		checkCertificateFiles(cfg, opts.Now()),
		checkAdminPermissions(cfg),
		checkTemporalSystem(cfg),
		checkAllowedNamespaceSide(cfg),
		checkNamespaceMappings(cfg),
		checkReplicationEndpoints(cfg),
		checkConnectionNames(cfg),
		checkNumericSettings(cfg),
		effectiveConfig(cfg),
		binaryIdentity(version, pod),
		checkConfigFreshness(configPath, opts.ProcessStartTime),
		checkLocalListeners(cfg, opts.ProcessStartTime, opts.CheckListener),
	)

	return report
}

func defaultProxyCheckOptions(opts ProxyCheckOptions) ProxyCheckOptions {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.Hostname == nil {
		opts.Hostname = os.Hostname
	}
	if opts.ProcessStartTime == nil {
		opts.ProcessStartTime = proxyProcessStartTime
	}
	if opts.CheckListener == nil {
		opts.CheckListener = checkListener
	}
	return opts
}

func parseSingleYAMLDocument(data []byte) error {
	if len(bytes.TrimSpace(data)) == 0 {
		return errors.New("config file is empty")
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := decoder.Decode(&doc); err != nil {
		return err
	}

	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("config contains more than one YAML document")
	}
	return nil
}

func checkConfigStructure(cfg config.S2SProxyConfig) Result {
	problems := splitErrorLines(cfg.Validate())
	if len(cfg.ClusterConnections) == 0 {
		problems = append(problems, "clusterConnections must contain at least one connection")
	}

	for i, conn := range cfg.ClusterConnections {
		prefix := fmt.Sprintf("clusterConnections[%d]", i)
		if conn.Local.ConnectionType != config.ConnTypeTCP {
			problems = append(problems, fmt.Sprintf("%s.local.connectionType must be tcp for the customer-side proxy", prefix))
		}
		if conn.Remote.ConnectionType != config.ConnTypeMuxClient {
			problems = append(problems, fmt.Sprintf("%s.remote.connectionType must be mux-client for the customer-side proxy", prefix))
		}

		problems = append(problems,
			dialAddressProblems(prefix+".local.tcpClient.address", conn.Local.TcpClient.ConnectionString)...)
		problems = append(problems,
			listenAddressProblems(prefix+".local.tcpServer.address", conn.Local.TcpServer.ConnectionString)...)
		problems = append(problems,
			dialAddressProblems(prefix+".remote.muxAddressInfo.address", conn.Remote.MuxAddressInfo.ConnectionString)...)
		if isAddressPlaceholder(conn.Remote.MuxAddressInfo.ConnectionString) {
			problems = append(problems, prefix+".remote.muxAddressInfo.address still contains a template placeholder")
		}
		problems = append(problems, healthCheckProblems(prefix+".remoteClusterHealthCheck", conn.RemoteClusterHealthCheck)...)
		problems = append(problems, healthCheckProblems(prefix+".localClusterHealthCheck", conn.LocalClusterHealthCheck)...)
	}

	if cfg.Metrics != nil && cfg.Metrics.Prometheus.ListenAddress != "" {
		problems = append(problems, listenAddressProblems("metrics.prometheus.listenAddress", cfg.Metrics.Prometheus.ListenAddress)...)
	}
	if cfg.ProfilingConfig != nil && cfg.ProfilingConfig.PProfHTTPAddress != "" {
		problems = append(problems, listenAddressProblems("profiling.pprofAddress", cfg.ProfilingConfig.PProfHTTPAddress)...)
	}
	problems = append(problems, duplicateListenerProblems(cfg)...)

	if len(problems) > 0 {
		return Result{Status: StatusFail, Name: "config structure is not usable", Details: problems}
	}
	return Result{Status: StatusPass, Name: "config format and structure are supported"}
}

func healthCheckProblems(path string, health config.HealthCheckConfig) []string {
	if health.ListenAddress == "" && health.Protocol == "" {
		return nil
	}
	var problems []string
	if health.Protocol != config.HTTP {
		problems = append(problems, fmt.Sprintf("%s.protocol must be http", path))
	}
	problems = append(problems, listenAddressProblems(path+".listenAddress", health.ListenAddress)...)
	return problems
}

func dialAddressProblems(path string, address string) []string {
	return hostPortProblems(path, address, true)
}

func listenAddressProblems(path string, address string) []string {
	return hostPortProblems(path, address, false)
}

func hostPortProblems(path string, address string, requireHost bool) []string {
	if address == "" {
		return []string{path + " is required"}
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || strings.ContainsAny(host, "/") || (requireHost && host == "") {
		return []string{path + " is not a usable host:port"}
	}
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p > 65535 {
		return []string{path + " is not a usable host:port"}
	}
	return nil
}

func duplicateListenerProblems(cfg config.S2SProxyConfig) []string {
	seen := make(map[string]string)
	var problems []string
	add := func(path string, address string) {
		if address == "" {
			return
		}
		key := normalizedListenerAddress(address)
		if previous, ok := seen[key]; ok {
			problems = append(problems, fmt.Sprintf("%s and %s use the same listener address %q", previous, path, address))
			return
		}
		seen[key] = path
	}
	for i, conn := range cfg.ClusterConnections {
		prefix := fmt.Sprintf("clusterConnections[%d]", i)
		add(prefix+".local.tcpServer.address", conn.Local.TcpServer.ConnectionString)
		if i == 0 {
			add(prefix+".remoteClusterHealthCheck.listenAddress", conn.RemoteClusterHealthCheck.ListenAddress)
			add(prefix+".localClusterHealthCheck.listenAddress", conn.LocalClusterHealthCheck.ListenAddress)
		}
	}
	if cfg.Metrics != nil {
		add("metrics.prometheus.listenAddress", cfg.Metrics.Prometheus.ListenAddress)
	}
	if cfg.ProfilingConfig != nil {
		add("profiling.pprofAddress", cfg.ProfilingConfig.PProfHTTPAddress)
	}
	return problems
}

func normalizedListenerAddress(address string) string {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return address
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "*"
	}
	return net.JoinHostPort(strings.ToLower(host), port)
}

type tlsLocation struct {
	path            string
	config          encryption.TLSConfig
	required        bool
	client          bool
	requireIdentity bool
}

func tlsLocations(cfg config.S2SProxyConfig) []tlsLocation {
	var locations []tlsLocation
	for i, conn := range cfg.ClusterConnections {
		prefix := fmt.Sprintf("clusterConnections[%d]", i)
		locations = append(locations,
			tlsLocation{path: prefix + ".local.tcpClient.tls", config: conn.Local.TcpClient.TLSConfig, client: true},
			tlsLocation{path: prefix + ".local.tcpServer.tls", config: conn.Local.TcpServer.TLSConfig, requireIdentity: true},
			tlsLocation{path: prefix + ".remote.muxAddressInfo.tls", config: conn.Remote.MuxAddressInfo.TLSConfig, required: true, client: true, requireIdentity: true},
		)
	}
	return locations
}

func checkTLSConfiguration(cfg config.S2SProxyConfig) Result {
	if len(cfg.ClusterConnections) == 0 {
		return Result{Status: StatusSkip, Name: "TLS cannot be checked without a cluster connection"}
	}
	var problems []string
	for _, location := range tlsLocations(cfg) {
		tlsCfg := location.config
		configured := hasTLSValue(tlsCfg)
		if location.required && !configured {
			problems = append(problems, location.path+" is required for the customer-side cloud connection")
			continue
		}
		if !configured {
			continue
		}
		if (tlsCfg.CertificatePath == "") != (tlsCfg.KeyPath == "") {
			problems = append(problems, location.path+" requires both certificatePath and keyPath")
		}
		if location.requireIdentity && (tlsCfg.CertificatePath == "" || tlsCfg.KeyPath == "") {
			problems = append(problems, location.path+" requires a certificate and key")
		}
		if location.client && !tlsCfg.SkipCAVerification && tlsCfg.CAServerName == "" {
			problems = append(problems, location.path+".caServerName is required when skipCAVerification is false")
		}
		if !location.client && !tlsCfg.SkipCAVerification && tlsCfg.RemoteCAPath == "" {
			problems = append(problems, location.path+".remoteCAPath is required when client certificate verification is enabled")
		}
	}

	if len(problems) > 0 {
		return Result{Status: StatusFail, Name: "TLS configuration is incomplete", Details: problems}
	}
	return Result{Status: StatusPass, Name: "TLS is configured for the remote connection"}
}

func hasTLSValue(cfg encryption.TLSConfig) bool {
	return cfg.CertificatePath != "" || cfg.KeyPath != "" || cfg.RemoteCAPath != "" ||
		cfg.CAServerName != ""
}

func checkCertificateFiles(cfg config.S2SProxyConfig, now time.Time) Result {
	var failures []string
	var warnings []string
	var details []string
	checked := 0
	incompletePairs := 0

	for _, location := range tlsLocations(cfg) {
		tlsCfg := location.config
		if (tlsCfg.CertificatePath == "") != (tlsCfg.KeyPath == "") {
			incompletePairs++
		}
		if tlsCfg.CertificatePath != "" && tlsCfg.KeyPath != "" {
			checked++
			pair, err := tls.LoadX509KeyPair(tlsCfg.CertificatePath, tlsCfg.KeyPath)
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s certificate/key: %v", location.path, err))
			} else if len(pair.Certificate) == 0 {
				failures = append(failures, location.path+" certificate file contains no certificate")
			} else {
				certificate, err := x509.ParseCertificate(pair.Certificate[0])
				if err != nil {
					failures = append(failures, fmt.Sprintf("%s certificate: %v", location.path, err))
				} else {
					remaining := certificate.NotAfter.Sub(now)
					switch {
					case now.Before(certificate.NotBefore):
						failures = append(failures, fmt.Sprintf("%s certificate is not valid until %s", location.path, certificate.NotBefore.Format(time.RFC3339)))
					case remaining <= 0:
						failures = append(failures, fmt.Sprintf("%s certificate expired at %s", location.path, certificate.NotAfter.Format(time.RFC3339)))
					case remaining <= certificateWarningAge:
						warnings = append(warnings, fmt.Sprintf("%s certificate expires in %s", location.path, remaining.Round(time.Hour)))
					default:
						details = append(details, fmt.Sprintf("%s certificate expires in %s", location.path, remaining.Round(24*time.Hour)))
					}
				}
			}
		}

		if tlsCfg.RemoteCAPath == "" {
			continue
		}
		if strings.HasPrefix(tlsCfg.RemoteCAPath, "http://") {
			failures = append(failures, location.path+".remoteCAPath uses HTTP; only a local file or HTTPS URL is supported")
			continue
		}
		if strings.HasPrefix(tlsCfg.RemoteCAPath, "https://") {
			warnings = append(warnings, location.path+".remoteCAPath is a URL and was not fetched by this local check")
			continue
		}
		checked++
		if err := validateCAFile(tlsCfg.RemoteCAPath); err != nil {
			failures = append(failures, fmt.Sprintf("%s.remoteCAPath: %v", location.path, err))
		}
	}

	if len(failures) > 0 {
		return Result{Status: StatusFail, Name: "certificate files are invalid", Details: append(failures, warnings...)}
	}
	if len(warnings) > 0 {
		return Result{Status: StatusWarn, Name: "certificate files need review", Details: append(warnings, details...)}
	}
	if checked == 0 {
		if incompletePairs > 0 {
			return Result{Status: StatusSkip, Name: "certificate pairs cannot be loaded until both paths are configured"}
		}
		return Result{Status: StatusSkip, Name: "no certificate files are configured"}
	}
	return Result{Status: StatusPass, Name: "certificate files load", Details: details}
}

func validateCAFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return encryption.ValidateCABundle(data, path)
}

func checkAdminPermissions(cfg config.S2SProxyConfig) Result {
	if len(cfg.ClusterConnections) == 0 {
		return Result{Status: StatusSkip, Name: "admin permissions cannot be checked without a cluster connection"}
	}
	var missing []string
	restricted := 0
	for i, conn := range cfg.ClusterConnections {
		if conn.ACLPolicy == nil || len(conn.ACLPolicy.AllowedMethods.AdminService) == 0 {
			continue
		}
		restricted++
		for _, method := range requiredAdminMethods {
			if !slices.Contains(conn.ACLPolicy.AllowedMethods.AdminService, method) {
				missing = append(missing, fmt.Sprintf("clusterConnections[%d] is missing %s", i, method))
			}
		}
	}

	if len(missing) > 0 {
		return Result{Status: StatusFail, Name: "admin permissions are incomplete", Details: missing}
	}
	if restricted == 0 {
		return Result{Status: StatusSkip, Name: "admin methods are unrestricted"}
	}
	return Result{Status: StatusPass, Name: "admin permissions include all required migration methods"}
}

func checkTemporalSystem(cfg config.S2SProxyConfig) Result {
	if len(cfg.ClusterConnections) == 0 {
		return Result{Status: StatusSkip, Name: "allowed namespaces cannot be checked without a cluster connection"}
	}
	var missing []string
	restricted := 0
	for i, conn := range cfg.ClusterConnections {
		if conn.ACLPolicy == nil || len(conn.ACLPolicy.AllowedNamespaces) == 0 {
			continue
		}
		restricted++
		if !slices.Contains(conn.ACLPolicy.AllowedNamespaces, temporalSystem) {
			missing = append(missing, fmt.Sprintf("clusterConnections[%d].aclPolicy.allowedNamespaces", i))
		}
	}

	if len(missing) > 0 {
		return Result{Status: StatusFail, Name: "temporal-system is not allowed", Details: missing}
	}
	if restricted == 0 {
		return Result{Status: StatusSkip, Name: "namespaces are unrestricted"}
	}
	return Result{Status: StatusPass, Name: "temporal-system is allowed"}
}

func checkAllowedNamespaceSide(cfg config.S2SProxyConfig) Result {
	if len(cfg.ClusterConnections) == 0 {
		return Result{Status: StatusSkip, Name: "allowed namespace side cannot be checked without a cluster connection"}
	}
	var wrong []string
	applicable := false
	for i, conn := range cfg.ClusterConnections {
		if conn.ACLPolicy == nil || len(conn.ACLPolicy.AllowedNamespaces) == 0 || len(conn.NamespaceTranslation.Mappings) == 0 {
			continue
		}
		applicable = true
		for _, mapping := range conn.NamespaceTranslation.Mappings {
			if mapping.Local != mapping.Remote && slices.Contains(conn.ACLPolicy.AllowedNamespaces, mapping.Remote) {
				wrong = append(wrong, fmt.Sprintf("clusterConnections[%d] allows remote name %q; use local name %q", i, mapping.Remote, mapping.Local))
			}
		}
	}

	if len(wrong) > 0 {
		return Result{Status: StatusFail, Name: "allowed namespaces use the wrong side", Details: wrong}
	}
	if !applicable {
		return Result{Status: StatusSkip, Name: "allowed namespace side cannot be compared"}
	}
	return Result{Status: StatusPass, Name: "allowed namespaces use customer-side names"}
}

func checkNamespaceMappings(cfg config.S2SProxyConfig) Result {
	if len(cfg.ClusterConnections) == 0 {
		return Result{Status: StatusSkip, Name: "namespace mappings cannot be checked without a cluster connection"}
	}
	var failures []string
	var warnings []string
	applicable := false
	for i, conn := range cfg.ClusterConnections {
		prefix := fmt.Sprintf("clusterConnections[%d].namespaceTranslation", i)
		if len(conn.NamespaceTranslation.Mappings) > 0 {
			applicable = true
		}
		for j, mapping := range conn.NamespaceTranslation.Mappings {
			path := fmt.Sprintf("%s.mappings[%d]", prefix, j)
			if mapping.Local == "" || mapping.Remote == "" {
				failures = append(failures, path+" requires both local and remote names")
			}
			if isNamespacePlaceholder(mapping.Local) || isNamespacePlaceholder(mapping.Remote) {
				failures = append(failures, path+" still contains a template placeholder")
			}
		}
		if _, err := conn.NamespaceTranslation.AsLocalToRemoteBiMap(); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", prefix, err))
		}

		if conn.ACLPolicy != nil && len(conn.ACLPolicy.AllowedNamespaces) > 0 {
			for _, namespace := range conn.ACLPolicy.AllowedNamespaces {
				if namespace == temporalSystem {
					continue
				}
				if !slices.ContainsFunc(conn.NamespaceTranslation.Mappings, func(mapping config.StringMapping) bool {
					return mapping.Local == namespace
				}) {
					warnings = append(warnings, fmt.Sprintf("%s has no explicit mapping for allowed namespace %q; this is valid only when both sides use the same name", prefix, namespace))
				}
			}
		}
	}

	if len(failures) > 0 {
		return Result{Status: StatusFail, Name: "namespace mappings are unusable", Details: append(failures, warnings...)}
	}
	if len(warnings) > 0 {
		return Result{Status: StatusWarn, Name: "namespace mappings need review", Details: warnings}
	}
	if !applicable {
		return Result{Status: StatusSkip, Name: "no namespace mappings are configured"}
	}
	return Result{Status: StatusPass, Name: "namespace mappings are internally consistent"}
}

func isNamespacePlaceholder(value string) bool {
	return value == "myNamespace" || value == "myNamespace.accountid" || value == "my-local" || value == "my-cloud.acct"
}

func isAddressPlaceholder(value string) bool {
	return value == "remote_proxy_service:8233" || value == "address-of-your-s2s-proxy-deployment:9233"
}

func checkReplicationEndpoints(cfg config.S2SProxyConfig) Result {
	if len(cfg.ClusterConnections) == 0 {
		return Result{Status: StatusSkip, Name: "replication endpoint cannot be checked without a cluster connection"}
	}
	var failures []string
	var warnings []string
	for i, conn := range cfg.ClusterConnections {
		path := fmt.Sprintf("clusterConnections[%d].replicationEndpoint", i)
		if isAddressPlaceholder(conn.ReplicationEndpoint) {
			failures = append(failures, path+" still contains a template placeholder")
			continue
		}
		host, port, err := splitUsableHostPort(conn.ReplicationEndpoint)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s %v", path, err))
			continue
		}
		if host == "localhost" || isLoopback(host) {
			warnings = append(warnings, fmt.Sprintf("%s uses loopback address %q", path, host))
		}

		_, listenerPort, err := net.SplitHostPort(conn.Local.TcpServer.ConnectionString)
		if err == nil && listenerPort != port {
			warnings = append(warnings, fmt.Sprintf("%s port %s differs from local listener port %s; this may be valid when internal routing translates ports", path, port, listenerPort))
		}
	}

	if len(failures) > 0 {
		return Result{Status: StatusFail, Name: "replication endpoint is unusable", Details: append(failures, warnings...)}
	}
	if len(warnings) > 0 {
		return Result{Status: StatusWarn, Name: "replication endpoint needs review", Details: warnings}
	}
	return Result{Status: StatusPass, Name: "replication endpoint is syntactically usable", Details: []string{"routability is not verified by this local check"}}
}

func splitUsableHostPort(address string) (string, string, error) {
	if address == "" {
		return "", "", errors.New("is required")
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" {
		return "", "", errors.New("is not a usable host:port")
	}
	p, err := strconv.Atoi(port)
	if err != nil || p <= 0 || p > 65535 {
		return "", "", errors.New("is not a usable host:port")
	}
	return host, port, nil
}

func isLoopback(host string) bool {
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func checkConnectionNames(cfg config.S2SProxyConfig) Result {
	if len(cfg.ClusterConnections) == 0 {
		return Result{Status: StatusSkip, Name: "connection names cannot be checked without a cluster connection"}
	}
	seen := make(map[string]struct{}, len(cfg.ClusterConnections))
	var problems []string
	for i, conn := range cfg.ClusterConnections {
		if conn.Name == "" {
			problems = append(problems, fmt.Sprintf("clusterConnections[%d].name is required", i))
			continue
		}
		if _, duplicate := seen[conn.Name]; duplicate {
			problems = append(problems, fmt.Sprintf("duplicate connection name %q", conn.Name))
		}
		seen[conn.Name] = struct{}{}
	}
	if len(problems) > 0 {
		return Result{Status: StatusFail, Name: "connection names are not unique", Details: problems}
	}
	return Result{Status: StatusPass, Name: "connection names are unique"}
}

func checkNumericSettings(cfg config.S2SProxyConfig) Result {
	if len(cfg.ClusterConnections) == 0 {
		return Result{Status: StatusSkip, Name: "numeric settings cannot be checked without a cluster connection"}
	}
	var failures []string
	var warnings []string
	applicable := false
	for i, conn := range cfg.ClusterConnections {
		path := fmt.Sprintf("clusterConnections[%d].shardCount", i)
		shards := conn.ShardCountConfig
		if conn.Remote.MuxCount < 0 {
			failures = append(failures, fmt.Sprintf("clusterConnections[%d].remote.muxCount cannot be negative", i))
		}

		switch shards.Mode {
		case config.ShardCountDefault:
			if shards.LocalShardCount != 0 || shards.RemoteShardCount != 0 {
				warnings = append(warnings, path+" counts are configured but mode is empty, so the values are inactive")
			}
		case config.ShardCountLCM, config.ShardCountRouting:
			applicable = true
			if shards.LocalShardCount <= 0 || shards.RemoteShardCount <= 0 {
				failures = append(failures, path+" requires positive localShardCount and remoteShardCount when mode is active")
			} else if shards.Mode == config.ShardCountLCM && lcmOverflowsInt32(shards.LocalShardCount, shards.RemoteShardCount) {
				failures = append(failures, path+" least common multiple exceeds the supported integer range")
			}
		default:
			failures = append(failures, fmt.Sprintf("%s.mode %q is not supported", path, shards.Mode))
		}
	}

	if len(failures) > 0 {
		return Result{Status: StatusFail, Name: "numeric settings are unusable", Details: append(failures, warnings...)}
	}
	if len(warnings) > 0 {
		return Result{Status: StatusWarn, Name: "numeric settings are configured but inactive", Details: warnings}
	}
	if !applicable {
		return Result{Status: StatusSkip, Name: "no shard-count translation is configured"}
	}
	return Result{Status: StatusPass, Name: "numeric settings are active and usable"}
}

func lcmOverflowsInt32(a int32, b int32) bool {
	gcd := func(x int64, y int64) int64 {
		for y != 0 {
			x, y = y, x%y
		}
		return x
	}
	result := int64(a) / gcd(int64(a), int64(b)) * int64(b)
	return result > math.MaxInt32
}

func effectiveConfig(cfg config.S2SProxyConfig) Result {
	var details []string
	for _, conn := range cfg.ClusterConnections {
		name := fmt.Sprintf("connection %q", conn.Name)
		details = append(details,
			fmt.Sprintf("%s: customer cluster dial=%s %s, TLS=%s", name, conn.Local.ConnectionType, conn.Local.TcpClient.ConnectionString, tlsState(conn.Local.TcpClient.TLSConfig)),
			fmt.Sprintf("%s: customer callback listen=%s, TLS=%s, advertised=%s", name, conn.Local.TcpServer.ConnectionString, tlsState(conn.Local.TcpServer.TLSConfig), conn.ReplicationEndpoint),
			fmt.Sprintf("%s: remote proxy dial=%s %s, muxCount=%d, TLS=%s", name, conn.Remote.ConnectionType, conn.Remote.MuxAddressInfo.ConnectionString, mux.DesiredMuxCount(conn.Remote), tlsState(conn.Remote.MuxAddressInfo.TLSConfig)),
		)

		shardMode := string(conn.ShardCountConfig.Mode)
		if shardMode == "" {
			shardMode = "none"
		}
		details = append(details, fmt.Sprintf(
			"%s: shardMode=%s, localShardCount=%d, remoteShardCount=%d, localFVI=%d, remoteFVI=%d",
			name,
			shardMode,
			conn.ShardCountConfig.LocalShardCount,
			conn.ShardCountConfig.RemoteShardCount,
			conn.FVITranslation.Local,
			conn.FVITranslation.Remote,
		))

		if conn.ACLPolicy == nil || len(conn.ACLPolicy.AllowedMethods.AdminService) == 0 {
			details = append(details, name+": admin methods=unrestricted")
		} else {
			details = append(details, fmt.Sprintf("%s: admin methods=%d configured", name, len(conn.ACLPolicy.AllowedMethods.AdminService)))
		}
		if conn.ACLPolicy == nil || len(conn.ACLPolicy.AllowedNamespaces) == 0 {
			details = append(details, name+": namespaces=unrestricted")
		} else {
			details = append(details, fmt.Sprintf("%s: allowed namespaces=%s", name, strings.Join(conn.ACLPolicy.AllowedNamespaces, ",")))
		}
		for _, mapping := range conn.NamespaceTranslation.Mappings {
			details = append(details, fmt.Sprintf("%s: namespace %s -> %s", name, mapping.Local, mapping.Remote))
		}
	}
	if cfg.Metrics != nil {
		details = append(details, "metrics="+cfg.Metrics.Prometheus.ListenAddress)
	}
	if cfg.ProfilingConfig != nil {
		details = append(details, "profiling="+cfg.ProfilingConfig.PProfHTTPAddress)
	}
	return Result{Status: StatusInfo, Name: "effective config", Details: details}
}

func tlsState(cfg encryption.TLSConfig) string {
	if !cfg.IsEnabled() {
		return "disabled"
	}
	if cfg.SkipCAVerification {
		return "enabled, verification skipped"
	}
	trust := "system roots"
	if cfg.RemoteCAPath != "" {
		trust = cfg.RemoteCAPath
	}
	if cfg.CAServerName != "" {
		return fmt.Sprintf("enabled, trust=%s, serverName=%s", trust, cfg.CAServerName)
	}
	return "enabled, trust=" + trust
}

func binaryIdentity(version string, pod string) Result {
	return Result{Status: StatusInfo, Name: "running binary identity", Details: []string{"version=" + version, "pod=" + pod}}
}

func checkConfigFreshness(configPath string, processStartTime func() (time.Time, error)) Result {
	info, err := os.Stat(configPath)
	if err != nil {
		return Result{Status: StatusUnknown, Name: "config freshness could not be checked", Details: []string{err.Error()}}
	}
	startedAt, err := processStartTime()
	if err != nil {
		return Result{Status: StatusUnknown, Name: "config freshness could not be checked", Details: []string{err.Error()}}
	}
	if info.ModTime().After(startedAt) {
		return Result{
			Status: StatusWarn,
			Name:   "config file changed after the proxy started",
			Details: []string{
				"proxy started: " + startedAt.Format(time.RFC3339),
				"config changed: " + info.ModTime().Format(time.RFC3339),
				"restart may be required",
			},
		}
	}
	return Result{Status: StatusPass, Name: "config file is unchanged since proxy startup"}
}

func proxyProcessStartTime() (time.Time, error) {
	cmdline, err := os.ReadFile("/proc/1/cmdline")
	if err != nil {
		return time.Time{}, errors.New("PID 1 proxy process is not available")
	}
	args := strings.FieldsFunc(string(cmdline), func(r rune) bool { return r == 0 })
	if len(args) < 2 || !strings.Contains(args[0], "s2s-proxy") || args[1] != "start" {
		return time.Time{}, errors.New("PID 1 is not s2s-proxy start")
	}

	proc, err := procfs.NewProc(1)
	if err != nil {
		return time.Time{}, fmt.Errorf("inspect PID 1: %w", err)
	}
	stat, err := proc.Stat()
	if err != nil {
		return time.Time{}, fmt.Errorf("inspect PID 1: %w", err)
	}
	startedAt, err := stat.StartTime()
	if err != nil {
		return time.Time{}, fmt.Errorf("inspect PID 1 start time: %w", err)
	}
	seconds, fraction := math.Modf(startedAt)
	return time.Unix(int64(seconds), int64(fraction*float64(time.Second))), nil
}

func checkLocalListeners(
	cfg config.S2SProxyConfig,
	processStartTime func() (time.Time, error),
	check func(string) error,
) Result {
	if _, err := processStartTime(); err != nil {
		return Result{Status: StatusUnknown, Name: "local listeners could not be checked", Details: []string{err.Error()}}
	}

	addresses := configuredListenerAddresses(cfg)
	if len(addresses) == 0 {
		return Result{Status: StatusSkip, Name: "no local listeners are configured"}
	}

	var failures []string
	for _, address := range addresses {
		if err := check(address); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", address, err))
		}
	}
	if len(failures) > 0 {
		return Result{Status: StatusFail, Name: "configured local listeners are not all reachable", Details: failures}
	}
	return Result{Status: StatusPass, Name: "configured local listeners are reachable", Details: addresses}
}

func configuredListenerAddresses(cfg config.S2SProxyConfig) []string {
	seen := make(map[string]struct{})
	var addresses []string
	add := func(address string) {
		if address == "" {
			return
		}
		if _, ok := seen[address]; ok {
			return
		}
		seen[address] = struct{}{}
		addresses = append(addresses, address)
	}
	for _, conn := range cfg.ClusterConnections {
		add(conn.Local.TcpServer.ConnectionString)
	}
	if len(cfg.ClusterConnections) > 0 {
		add(cfg.ClusterConnections[0].RemoteClusterHealthCheck.ListenAddress)
		add(cfg.ClusterConnections[0].LocalClusterHealthCheck.ListenAddress)
	}
	if cfg.Metrics != nil {
		add(cfg.Metrics.Prometheus.ListenAddress)
	}
	if cfg.ProfilingConfig != nil {
		add(cfg.ProfilingConfig.PProfHTTPAddress)
	}
	return addresses
}

func checkListener(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), defaultListenerTimeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

func splitErrorLines(err error) []string {
	if err == nil {
		return nil
	}
	return strings.Split(err.Error(), "\n")
}
