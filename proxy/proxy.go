package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"slices"
	"strings"
	"time"

	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/temporalio/s2s-proxy/adminplane"
	proxyadminv1 "github.com/temporalio/s2s-proxy/api/proxyadmin/v1"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/logging"
	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	migrationId struct {
		name string
		// Needs some config revision before uncommenting:
		//accountId string
	}

	Proxy struct {
		lifetime                context.Context
		cancel                  context.CancelFunc
		localHealthCheckConfig  *config.HealthCheckConfig
		remoteHealthCheckConfig *config.HealthCheckConfig
		metricsConfig           *config.MetricsConfig
		profilingConfig         *config.ProfilingConfig
		proxyAdminConfig        config.ProxyAdminConfig
		clusterConnections      map[migrationId]*ClusterConnection
		localHealthCheckServer  *http.Server
		remoteHealthCheckServer *http.Server
		metricsServer           *http.Server
		adminServers            []*adminplane.Server
		adminService            proxyadminv1.ProxyAdminServiceServer
		version                 string
		memberID                string
		startTime               time.Time
		logProvider             logging.LoggerProvider
	}

	// Identity is who this process says it is in admin responses.
	//
	// MemberID must differ between processes.
	// It is supplied rather than read from the shared config file.
	// A value in that file would be identical on every replica.
	// Deduplication by id would then collapse the whole deployment into a single member.
	Identity struct {
		Version  string
		MemberID string
	}
)

// DefaultIdentity derives this process's identity from its environment.
// $POD_NAME comes first.
// A Kubernetes deployment can then be explicit.
// The hostname is the fallback.
// Inside a pod that is the pod name.
func DefaultIdentity(version string) Identity {
	id := Identity{Version: version}
	if name := os.Getenv("POD_NAME"); name != "" {
		id.MemberID = name
		return id
	}
	if host, err := os.Hostname(); err == nil {
		id.MemberID = host
	}
	return id
}

func NewProxy(configProvider config.ConfigProvider, logProvider logging.LoggerProvider, identity Identity) (*Proxy, error) {
	s2sConfig := configProvider.GetS2SProxyConfig()
	if err := s2sConfig.Validate(); err != nil {
		return nil, fmt.Errorf("cannot create proxy: invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	proxy := &Proxy{
		lifetime:           ctx,
		cancel:             cancel,
		clusterConnections: make(map[migrationId]*ClusterConnection, len(s2sConfig.ClusterConnections)),
		logProvider:        logProvider,
		version:            identity.Version,
		memberID:           identity.MemberID,
		startTime:          time.Now(),
	}
	if len(s2sConfig.ClusterConnections) == 0 {
		cancel()
		return nil, errors.New("cannot create proxy: clusterConnections is empty")
	}
	if s2sConfig.Metrics != nil {
		proxy.metricsConfig = s2sConfig.Metrics
	}
	proxy.profilingConfig = s2sConfig.ProfilingConfig
	proxy.proxyAdminConfig = s2sConfig.ProxyAdmin

	plane, err := proxy.newAdminPlane()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("cannot create proxy: %w", err)
	}
	proxy.adminService = NewProxyAdminServiceServer(proxy, plane)

	for _, clusterCfg := range s2sConfig.ClusterConnections {
		id := migrationId{clusterCfg.Name}
		// The name identifies the connection to the admin API and selects which peer a forwarded call reaches.
		// A duplicate is not merely confusing.
		// The second connection would take over the first one's traffic silently.
		if _, duplicate := proxy.clusterConnections[id]; duplicate {
			// Earlier connections have already bound listeners and dialed clients.
			// Their cleanup hangs off the lifetime.
			// Cancel it or the ports stay bound.
			cancel()
			return nil, fmt.Errorf("cannot create proxy: duplicate cluster connection name %q", clusterCfg.Name)
		}
		cc, err := NewClusterConnection(ctx, clusterCfg, proxy.adminService, logProvider)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("cannot create cluster connection %q: %w", clusterCfg.Name, err)
		}
		proxy.clusterConnections[id] = cc
	}
	// TODO: correctly host multiple health checks
	if len(s2sConfig.ClusterConnections) > 0 && s2sConfig.ClusterConnections[0].LocalClusterHealthCheck.ListenAddress != "" {
		proxy.localHealthCheckConfig = &s2sConfig.ClusterConnections[0].LocalClusterHealthCheck
	}
	if len(s2sConfig.ClusterConnections) > 0 && s2sConfig.ClusterConnections[0].RemoteClusterHealthCheck.ListenAddress != "" {
		proxy.remoteHealthCheckConfig = &s2sConfig.ClusterConnections[0].RemoteClusterHealthCheck
	}

	metrics.NewProxyCount.Inc()
	return proxy, nil
}

// MemberID reports how this process identifies itself in admin responses.
func (s *Proxy) MemberID() string { return s.memberID }

func (s *Proxy) startHealthCheckHandler(lifetime context.Context, healthChecker HealthChecker, cfg config.HealthCheckConfig) (*http.Server, error) {
	if cfg.Protocol != config.HTTP {
		return nil, fmt.Errorf("unsupported health check protocol %s", cfg.Protocol)
	}

	// Set up the handler. Avoid the global ServeMux so that we can create N of these in unit test suites
	serveMux := http.NewServeMux()
	serveMux.HandleFunc("/health", healthChecker.createHandler())
	// Define the server and its settings
	healthCheckServer := &http.Server{
		Addr:    cfg.ListenAddress,
		Handler: serveMux,
	}

	go func() {
		s.logProvider.Get("init").Info("Starting health check server", tag.Address(cfg.ListenAddress))
		if err := healthCheckServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logProvider.Get("init").Error("Error starting server", tag.Error(err))
		}
	}()
	context.AfterFunc(lifetime, func() {
		_ = healthCheckServer.Close()
	})
	return healthCheckServer, nil
}

func (s *Proxy) startPProfHTTPServer() {
	if s.profilingConfig == nil || len(s.profilingConfig.PProfHTTPAddress) == 0 {
		return
	}
	addr := s.profilingConfig.PProfHTTPAddress
	logger := s.logProvider.Get("init")
	http.HandleFunc("/debug/connections", func(w http.ResponseWriter, r *http.Request) {
		HandleDebugInfo(w, r, s, logger)
	})
	go func() {
		logger.Info("Start pprof http server", tag.NewStringTag("address", addr))
		if err := http.ListenAndServe(addr, nil); err != nil {
			panic(err)
		}
	}()
}

// startAdminServers serves ProxyAdminService on the local operator listener and, when configured,
// on the peer listener that sibling pods use.
//
// The service is otherwise reachable only over a mux.
// A plain gRPC client cannot speak that.
//
// Neither listener is required for the proxy to do its job.
// A bind failure is logged and startup continues, as it does for metrics and health checks.
func (s *Proxy) startAdminServers() {
	if addr := s.proxyAdminConfig.ListenAddress; addr != "" {
		// Trusted local endpoint.
		// Reflection is registered so grpcurl works without a descriptor.
		s.startAdminServer(LogProxyAdmin, addr, adminplane.ServerOptions{Role: adminplane.RoleOperator},
			nil, true)
	}

	peerCfg := s.proxyAdminConfig.Peer
	if peerCfg == nil || peerCfg.ListenAddress == "" {
		return
	}
	var creds credentials.TransportCredentials
	if peerCfg.TLS != nil && peerCfg.TLS.IsEnabled() {
		tlsConfig, err := peerServerTLSConfig(*peerCfg.TLS)
		if err != nil {
			s.logProvider.Get(LogProxyAdminPeer).Error("Failed to build peer TLS config", tag.Error(err))
			return
		}
		creds = credentials.NewTLS(tlsConfig)
	}
	// No reflection.
	// This listener is reachable from the pod network and only ever talks to other pods of this deployment.
	// Those pods know the schema.
	s.startAdminServer(LogProxyAdminPeer, peerCfg.ListenAddress,
		adminplane.ServerOptions{Role: adminplane.RolePeer}, creds, false)
}

func (s *Proxy) startAdminServer(
	component logging.LogComponentName,
	address string,
	opts adminplane.ServerOptions,
	creds credentials.TransportCredentials,
	withReflection bool,
) {
	logger := s.logProvider.Get(component)
	name := string(component)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		logger.Error("Failed to listen for ProxyAdminService", tag.Address(address), tag.Error(err))
		return
	}
	serverOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(adminplane.UnaryInterceptor(opts)),
		grpc.ChainStreamInterceptor(adminplane.StreamInterceptor(opts)),
	}
	if creds != nil {
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}
	grpcServer := grpc.NewServer(serverOpts...)
	proxyadminv1.RegisterProxyAdminServiceServer(grpcServer, s.adminService)
	if withReflection {
		reflection.Register(grpcServer)
	}
	server := adminplane.NewServer(name, s.lifetime, listener, grpcServer, logger)
	s.adminServers = append(s.adminServers, server)
	server.Start()
}

// peerServerTLSConfig builds the peer listener's TLS config.
//
// encryption.GetServerTLSConfig is deliberately not reused.
// It sets RequireAnyClientCert and replaces VerifyPeerCertificate with a function that logs the subject and returns nil.
// The client chain is never checked.
//
// That is acceptable where TLS is only providing encryption.
// This listener is bound to the pod network.
// The certificate is the only thing distinguishing a sibling pod from anything else that can reach it.
func peerServerTLSConfig(cfg encryption.TLSConfig) (*tls.Config, error) {
	certificate, err := tls.LoadX509KeyPair(cfg.CertificatePath, cfg.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("loading peer key pair: %w", err)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		MinVersion:   tls.VersionTLS12,
	}
	if cfg.RemoteCAPath == "" {
		return nil, errors.New("peer tls requires remoteCAPath to verify sibling certificates")
	}
	pool, err := encryption.LoadCACert(cfg.RemoteCAPath)
	if err != nil {
		return nil, fmt.Errorf("loading peer CA: %w", err)
	}
	tlsConfig.ClientCAs = pool
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	return tlsConfig, nil
}

func (s *Proxy) startMetricsHandler(lifetime context.Context, cfg config.MetricsConfig) error {
	// Why not use the global ServeMux? So that it can be used in unit tests
	mux := http.NewServeMux()
	mux.Handle("/metrics", metrics.NewMetricsHandler(s.logProvider.Get("metrics")))
	s.metricsServer = &http.Server{
		Addr:    cfg.Prometheus.ListenAddress,
		Handler: mux,
	}

	go func() {
		s.logProvider.Get("metrics").Info("Starting metrics server", tag.Address(cfg.Prometheus.ListenAddress))
		if err := s.metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logProvider.Get("metrics").Error("Error starting server", tag.Error(err))
		}
	}()
	context.AfterFunc(lifetime, func() {
		_ = s.metricsServer.Close()
	})
	return nil
}

// SelfClusterConnections describes this process alone.
// It reports every cluster connection this process is configured with, as one member's contribution to a deployment-wide answer.
func (s *Proxy) SelfClusterConnections(context.Context) *proxyadminv1.DescribeClusterConnectionsResponse {
	resp := &proxyadminv1.DescribeClusterConnectionsResponse{}
	startTime := timestamppb.New(s.startTime)
	for _, cc := range s.clusterConnections {
		described := cc.describeAdmin(s.memberID, s.version)
		for _, m := range described.GetMembers() {
			m.GetIdentity().StartTime = startTime
		}
		resp.ClusterConnections = append(resp.ClusterConnections, described)
	}
	// Sorted so repeated calls return the same order.
	slices.SortFunc(resp.ClusterConnections, func(a, b *proxyadminv1.ClusterConnection) int {
		return strings.Compare(a.GetName(), b.GetName())
	})
	return resp
}

// PeerConn returns a client for the peer proxy of a named cluster connection.
//
// The two failure modes are distinguished because they mean different things to whoever is debugging.
// An unknown name is a typo.
// A known name that cannot carry a call is a link that is down.
//
// Reporting a down link immediately also avoids spending the whole forward budget on a resolver that will never produce an address.
func (s *Proxy) PeerConn(name string) (grpc.ClientConnInterface, error) {
	cc, ok := s.clusterConnections[migrationId{name}]
	if !ok {
		return nil, status.Errorf(codes.InvalidArgument, "unknown cluster connection %q", name)
	}
	if _, multiplexed := cc.inboundMux(); !multiplexed {
		// The admin service is only registered on a mux server.
		// Forwarding over a TCP connection would reach a server that never registered it and return a bare Unimplemented.
		return nil, status.Errorf(codes.FailedPrecondition,
			"cluster connection %q is not multiplexed", name)
	}
	if !cc.outboundClient.CanMakeCalls() {
		return nil, status.Errorf(codes.FailedPrecondition,
			"cluster connection %q has no established mux session", name)
	}
	return cc.outboundClient, nil
}

// newAdminPlane assembles the discovery and dialing an admin endpoint needs to answer for the whole deployment.
// Peer discovery is optional.
// Without it a proxy only ever describes itself.
func (s *Proxy) newAdminPlane() (*adminplane.Plane, error) {
	plane := &adminplane.Plane{
		Peers:     s,
		MemberID:  s.memberID,
		Discovery: adminplane.NoDiscovery(),
	}

	peerCfg := s.proxyAdminConfig.Peer
	if peerCfg == nil {
		return plane, nil
	}

	discovery, err := adminplane.NewDiscovery(peerCfg.Discovery, peerCfg.PeerPort())
	if err != nil {
		return nil, err
	}
	plane.Discovery = discovery

	// Built once.
	// GetClientTLSConfig re-reads the key pair from disk on every call.
	var peerTLS *tls.Config
	if peerCfg.TLS != nil && peerCfg.TLS.IsEnabled() {
		peerTLS, err = encryption.GetClientTLSConfig(*peerCfg.TLS)
		if err != nil {
			return nil, fmt.Errorf("peer client TLS: %w", err)
		}
	}
	plane.Dial = func(ctx context.Context, address string) (grpc.ClientConnInterface, func(), error) {
		return adminplane.DialOnce(address, peerTLS)
	}
	return plane, nil
}

func (s *Proxy) Start() error {
	s.startPProfHTTPServer()

	if s.localHealthCheckConfig != nil {
		var err error
		healthFn := func() bool {
			// TODO: Rethink health checks. The inbound/outbound traffic availability isn't quite right for a health check
			return true
		}
		if s.localHealthCheckServer, err = s.startHealthCheckHandler(s.lifetime, newLocalHealthCheck(healthFn, s.logProvider.Get("healthCheck")), *s.localHealthCheckConfig); err != nil {
			return err
		}
	} else {
		s.logProvider.Get("init").Warn("Started up without local cluster health check! Double-check the YAML config," +
			" it needs at least the following path: clusterConnections[].localClusterHealthCheck.listenAddress")
	}

	if s.remoteHealthCheckConfig != nil {
		healthFn := func() bool {
			// TODO: Rethink health checks. The inbound/outbound traffic availability isn't quite right for a health check
			return true
		}
		var err error
		if s.remoteHealthCheckServer, err = s.startHealthCheckHandler(s.lifetime, newRemoteHealthCheck(healthFn, s.logProvider.Get("healthCheck")), *s.remoteHealthCheckConfig); err != nil {
			return err
		}
	} else {
		s.logProvider.Get("init").Warn("Started up without remote cluster health check! Double-check the YAML config," +
			" it needs at least the following path: clusterConnections[].remoteClusterHealthCheck.listenAddress")
	}

	if s.metricsConfig != nil {
		if err := s.startMetricsHandler(s.lifetime, *s.metricsConfig); err != nil {
			return err
		}
	} else {
		s.logProvider.Get("init").Warn(`Started up without metrics! Double-check the YAML config,` +
			` it needs at least the following path: metrics.prometheus.listenAddress`)
	}

	s.startAdminServers()

	for _, v := range s.clusterConnections {
		v.Start()
	}

	s.logProvider.Get("init").Info(fmt.Sprintf("Started Proxy with the following config:\n%s", s.Describe()))

	return nil
}

func (s *Proxy) Stop() {
	// All parts of the Proxy watch the "lifetime" context. Cancelling it will close all components
	// where necessary
	s.cancel()
}

func (s *Proxy) Done() <-chan struct{} {
	return s.lifetime.Done()
}

func (s *Proxy) Describe() string {
	sb := strings.Builder{}
	sb.WriteString("[proxy.Proxy with cluster connections:\n\t")
	for k, v := range s.clusterConnections {
		sb.WriteString(fmt.Sprintf("%s:", k.name))
		sb.WriteString(v.Describe())
		sb.WriteString("\n\t")
	}
	sb.WriteString("]")
	return sb.String()
}
