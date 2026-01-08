package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"time"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/auth"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport/grpcutil"
	"github.com/temporalio/s2s-proxy/transport/mux"
)

type (
	// simpleGRPCServer is a self-sufficient package of listener, server, logger, and lifetime. It can be Started,
	// which creates a GoRoutine that listens on the provided listener using server.Serve until lifetime closes.
	simpleGRPCServer struct {
		name     string
		lifetime context.Context
		listener net.Listener
		server   *grpc.Server
		logger   log.Logger
	}

	// describableClientConn is a small extension to add a Describe() method to grpc.ClientConn. Used for logging.
	describableClientConn struct {
		*grpc.ClientConn
	}

	// ClusterConnection contains a bidirectional connection between a local Temporal server and a remote.
	ClusterConnection struct {
		// When lifetime closes, all clients and servers in ClusterConnection will stop.
		lifetime context.Context
		// outboundServer receives connections from the local Temporal and makes calls using outboundClient.
		outboundServer contextAwareServer
		// outboundClient is connected to a remote Temporal server somewhere.
		outboundClient closableClientConn
		// inboundServer receives connections from a remote Temporal server and calls using the inboundClient.
		inboundServer contextAwareServer
		// inboundClient talks to the local Temporal frontend.
		inboundClient    closableClientConn
		inboundObserver  *ReplicationStreamObserver
		outboundObserver *ReplicationStreamObserver
		logger           log.Logger
	}
	// contextAwareServer represents a startable gRPC server used to provide the Temporal interface on some connection.
	// IsUsable and Describe allow the caller to know and log the current state of the server.
	contextAwareServer interface {
		Start()
		Describe() string
		Name() string
		CanAcceptConnections() bool
	}
	// closableClientConn represents a ClientConnInterface with a Close and a Describe. It's implemented by
	// grpcutil.MultiClientConn and describableClientConn.
	closableClientConn interface {
		grpc.ClientConnInterface
		Close() error
		Describe() string
		CanMakeCalls() bool
	}
)

// NewTCPClusterConnection creates a ClusterConnection between two TCP-TLS endpoints. This requires open TCP ports from
// both clusters, which is rare outside a firewalled network or test environment. When configuring the proxy for WAN
// connections, consider using NewMuxClusterConnection.
func NewTCPClusterConnection(lifetime context.Context,
	inbound config.ProxyConfig,
	outbound config.ProxyConfig,
	namespaceNameTranslation config.NameTranslationConfig,
	searchAttributeTranslation config.SATranslationConfig,
	logger log.Logger) (*ClusterConnection, error) {
	cc := &ClusterConnection{
		lifetime: lifetime,
		logger:   logger,
	}
	var err error
	cc.inboundObserver = NewReplicationStreamObserver(logger)
	cc.inboundClient, cc.inboundServer, err = buildSimpleServerArc(lifetime, true, outbound.Server.ExternalAddress,
		inbound, "inbound", namespaceNameTranslation, searchAttributeTranslation, cc.inboundObserver, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build inbound config %s: %w", inbound.Name, err)
	}
	cc.outboundObserver = NewReplicationStreamObserver(logger)
	cc.outboundClient, cc.outboundServer, err = buildSimpleServerArc(lifetime, false, "",
		outbound, "outbound", namespaceNameTranslation, searchAttributeTranslation, cc.outboundObserver, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build outbound config %s: %w", outbound.Name, err)
	}
	return cc, nil
}

// buildSimpleServerArc creates a paired TCP server and client in a single direction. Two of these together create a
// TCP-based ClusterConnection.
func buildSimpleServerArc(lifetime context.Context, isInbound bool, overrideExternalAddress string,
	proxyCfg config.ProxyConfig, directionLabel string, namespaceNameTranslation config.NameTranslationConfig,
	searchAttributeTranslation config.SATranslationConfig, observer *ReplicationStreamObserver, logger log.Logger) (closableClientConn, contextAwareServer, error) {
	client, err := buildTLSTCPClient(lifetime, proxyCfg.Client.ServerAddress, proxyCfg.Client.TLS, directionLabel)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create %s tls client: %w", directionLabel, err)
	}

	listener, err := net.Listen("tcp", proxyCfg.Server.ListenAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid configuration for inbound server: %w", err)
	}
	serverCfg, err := buildProxyServer(client, isInbound, proxyCfg, overrideExternalAddress,
		namespaceNameTranslation, searchAttributeTranslation, observer, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create inbound server: %w", err)
	}
	server := &simpleGRPCServer{
		name:     proxyCfg.Name,
		lifetime: lifetime,
		listener: listener,
		server:   serverCfg,
		logger:   logger,
	}
	return client, server, nil
}

// buildTLSTCPClient creates a grpc.ClientConn using the provided configuration. It schedules a goroutine that closes
// the grpc.ClientConn when the provided lifetime ends.
func buildTLSTCPClient(lifetime context.Context, serverAddress string, tlsCfg encryption.ClientTLSConfig, directionLabel string) (closableClientConn, error) {
	var parsedTLSCfg *tls.Config
	if tlsCfg.IsEnabled() {
		var err error
		parsedTLSCfg, err = encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("config error when creating tls config: %w", err)
		}
	}
	client, err := grpc.NewClient(serverAddress, grpcutil.MakeDialOptions(parsedTLSCfg, metrics.GetGRPCClientMetrics(directionLabel))...)
	if err != nil {
		return nil, fmt.Errorf("could not create inbound client: %w", err)
	}
	context.AfterFunc(lifetime, func() {
		// grpc.ClientConn must be closed, but it's not context-aware. Make sure the client closes when the lifetime ends
		_ = client.Close()
	})
	return describableClientConn{client}, nil
}

// NewMuxClusterConnection creates a ClusterConnection based on a set of mux connections between the proxy host
// and some remote. It will call mux.NewGRPCMuxManager, which sets up a new mux.MultiMuxManager and configures it to
// dispatch a new grpc.Server instance on every newly established mux. For outbound communication,
// grpcutil.MultiClientConn is notified of the active muxes by the MultiMuxManager, allowing outbound traffic to
// round-robin on any available mux connection.
func NewMuxClusterConnection(lifetime context.Context,
	muxConfig config.MuxTransportConfig,
	inbound config.ProxyConfig,
	outbound config.ProxyConfig,
	namespaceNameTranslation config.NameTranslationConfig,
	searchAttributeTranslation config.SATranslationConfig,
	logger log.Logger) (*ClusterConnection, error) {
	cc := &ClusterConnection{
		lifetime: lifetime,
		logger:   logger,
	}
	var err error

	// Clients are built first because all proxy servers need a reference to the client of the same direction
	// (inbound servers need inbound client, outbound server needs outbound client)
	// The outbound "client" is actually one ClientConn on top of many Mux connections, implemented by MultiClientConn.
	// Check grpcutil.MultiClientConn for more info
	cc.outboundClient, err = grpcutil.NewMultiClientConn(lifetime, fmt.Sprintf("client-conn-%s", muxConfig.Name),
		grpcutil.MakeDialOptions(nil, metrics.GetGRPCClientMetrics("outbound"))...)
	if err != nil {
		return nil, fmt.Errorf("failed to create multi client connection for mux: %w", err)
	}
	// Inbound client is a plain TLS+TCP grpc client connection pointed at the Temporal frontend.
	cc.inboundClient, err = buildTLSTCPClient(lifetime, inbound.Client.ServerAddress, inbound.Client.TLS, "inbound")
	if err != nil {
		return nil, fmt.Errorf("failed to create inbound client: %w", err)
	}

	cc.inboundObserver = NewReplicationStreamObserver(logger)
	// Servers are built second because they require the clients to operate (since all they do is delegate to the matched client).
	//The inbound "server" is a config shared across N listeners on N different mux connections by the MultiMuxManager.
	inboundServerCfg, err := buildProxyServer(cc.inboundClient, true, inbound, outbound.Server.ExternalAddress,
		namespaceNameTranslation, searchAttributeTranslation, cc.inboundObserver, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create inbound server for config %s: %w", inbound.Name, err)
	}

	cc.inboundServer, err = mux.NewGRPCMuxManager(lifetime, muxConfig, cc.outboundClient.(*grpcutil.MultiClientConn), inboundServerCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create GRPC mux manager for config named %s: %w", muxConfig.Name, err)
	}

	cc.outboundObserver = NewReplicationStreamObserver(logger)
	// The outbound server is a plain old TCP listener for the local Temporal cluster
	outboundListener, err := net.Listen("tcp", outbound.Server.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on outbound server address %s: %w", outbound.Server.ListenAddress, err)
	}
	outboundServerCfg, err := buildProxyServer(cc.outboundClient, false, outbound, "",
		namespaceNameTranslation, searchAttributeTranslation, cc.outboundObserver, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create outbound server for config %s: %w", outbound.Name, err)
	}
	cc.outboundServer = &simpleGRPCServer{
		name:     outbound.Name,
		lifetime: lifetime,
		listener: outboundListener,
		server:   outboundServerCfg,
		logger:   logger,
	}

	return cc, nil
}

func (c *ClusterConnection) Start() {
	c.inboundServer.Start()
	c.inboundObserver.Start(c.lifetime, c.inboundServer.Name(), "inbound")
	c.outboundServer.Start()
	c.outboundObserver.Start(c.lifetime, c.outboundServer.Name(), "outbound")
}
func (c *ClusterConnection) Describe() string {
	return fmt.Sprintf("[ClusterConnection connects outbound server %s to outbound client %s, inbound server %s to inbound client %s]",
		c.outboundServer.Describe(), c.outboundClient.Describe(), c.inboundServer.Describe(), c.inboundClient.Describe())
}

func (c *ClusterConnection) AcceptingInboundTraffic() bool {
	return c.inboundClient.CanMakeCalls() && c.inboundServer.CanAcceptConnections()
}
func (c *ClusterConnection) AcceptingOutboundTraffic() bool {
	return c.outboundClient.CanMakeCalls() && c.outboundServer.CanAcceptConnections()
}

// buildProxyServer uses the provided grpc.ClientConnInterface and config.ProxyConfig to create a grpc.Server that proxies
// the Temporal API across the ClientConnInterface.
func buildProxyServer(client grpc.ClientConnInterface, isInbound bool, serverCfg config.ProxyConfig, overrideExternalAddress string,
	namespaceTranslation config.NameTranslationConfig, searchAttrTranslation config.SATranslationConfig, observer *ReplicationStreamObserver, logger log.Logger) (*grpc.Server, error) {
	directionLabel := "outbound"
	if isInbound {
		directionLabel = "inbound"
	}
	serverOpts, err := MakeServerOptions(logger, isInbound, serverCfg.Server.TLS, serverCfg.ACLPolicy, namespaceTranslation, searchAttrTranslation)
	if err != nil {
		return nil, fmt.Errorf("could not parse server options: %w", err)
	}
	server := grpc.NewServer(serverOpts...)
	adminServiceImpl := NewAdminServiceProxyServer(fmt.Sprintf("%sAdminService", directionLabel), adminservice.NewAdminServiceClient(client),
		overrideExternalAddress, serverCfg.APIOverrides, []string{directionLabel}, observer.ReportStreamValue, logger)
	var accessControl *auth.AccessControl
	if serverCfg.ACLPolicy != nil {
		accessControl = auth.NewAccesControl(serverCfg.ACLPolicy.AllowedNamespaces)
	}
	workflowServiceImpl := NewWorkflowServiceProxyServer("inboundWorkflowService", workflowservice.NewWorkflowServiceClient(client),
		accessControl, logger)
	adminservice.RegisterAdminServiceServer(server, adminServiceImpl)
	workflowservice.RegisterWorkflowServiceServer(server, workflowServiceImpl)
	return server, nil
}
func (s *simpleGRPCServer) Start() {
	metrics.GRPCServerStarted.WithLabelValues(s.name).Inc()
	go func() {
		s.logger.Info("Starting TCP-TLS gRPC server", tag.Name(s.name), tag.Address(s.listener.Addr().String()))
		for s.lifetime.Err() == nil {
			err := s.server.Serve(s.listener)
			if s.lifetime.Err() != nil {
				// Cluster is closing, just exit.
				return
			}
			if err != nil {
				s.logger.Warn("GRPC server failed", tag.NewStringTag("direction", "outbound"), tag.Address(s.listener.Addr().String()), tag.Error(err))
				if err == io.EOF {
					metrics.GRPCServerStopped.WithLabelValues(s.name, "eof").Inc()
				} else if !errors.Is(err, grpc.ErrServerStopped) {
					metrics.GRPCServerStopped.WithLabelValues(s.name, "unknown").Inc()
				}
			}
			time.Sleep(1 * time.Second)
		}
	}()
	// The basic net.Listen, grpc.Server, and ClientConn are not context-aware, so make sure they clean up on context close.
	context.AfterFunc(s.lifetime, func() {
		metrics.GRPCServerStopped.WithLabelValues(s.name, "none").Inc()
		s.server.GracefulStop()
		_ = s.listener.Close()
	})
}
func (s *simpleGRPCServer) CanAcceptConnections() bool {
	return true
}
func (s *simpleGRPCServer) CanMakeCalls() bool {
	return true
}
func (s *simpleGRPCServer) Describe() string {
	return fmt.Sprintf("[simpleGRPCServer %s listening on %s. lifetime.Err: %e]", s.name, s.listener.Addr(), s.lifetime.Err())
}
func (s *simpleGRPCServer) Name() string {
	return s.name
}
func (d describableClientConn) Describe() string {
	return fmt.Sprintf("[grpc.ClientConn %s, state=%s]", d.Target(), d.GetState().String())
}
func (d describableClientConn) CanMakeCalls() bool {
	return true
}
