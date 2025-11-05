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
	"github.com/temporalio/s2s-proxy/collect"
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

func NewClusterConnection(lifetime context.Context, connConfig config.ClusterConnConfig, logger log.Logger) (*ClusterConnection, error) {
	cc := &ClusterConnection{
		lifetime: lifetime,
		logger:   log.With(logger, tag.NewStringTag("clusterConn", connConfig.Name)),
	}
	var err error
	cc.inboundClient, err = createClient(lifetime, connConfig.Name, connConfig.LocalServer.Connection, "inbound")
	if err != nil {
		return nil, err
	}
	cc.outboundClient, err = createClient(lifetime, connConfig.Name, connConfig.RemoteServer.Connection, "outbound")
	if err != nil {
		return nil, err
	}
	nsTranslations, err := connConfig.NamespaceTranslation.AsLocalToRemoteBiMap()
	if err != nil {
		return nil, err
	}
	saTranslations, err := connConfig.SearchAttributeTranslation.AsLocalToRemoteSATranslation()
	if err != nil {
		return nil, err
	}
	cc.inboundServer, cc.inboundObserver, err = createServer(lifetime, connConfig.Name, connConfig.RemoteServer, "inbound",
		cc.inboundClient, cc.outboundClient, nsTranslations.Inverse(), saTranslations.Inverse(), cc.logger)
	if err != nil {
		return nil, err
	}
	cc.outboundServer, cc.outboundObserver, err = createServer(lifetime, connConfig.Name, connConfig.LocalServer, "outbound",
		cc.outboundClient, cc.inboundClient, nsTranslations, saTranslations, cc.logger)
	if err != nil {
		return nil, err
	}
	return cc, nil
}

func createClient(lifetime context.Context, connectionName string, transportCfg config.TransportInfo, directionLabel string) (closableClientConn, error) {
	switch transportCfg.ConnectionType {
	case config.ConnTypeTCP:
		return buildTLSTCPClient(lifetime, transportCfg.TcpClient.ConnectionString, transportCfg.TcpClient.TLSConfig, directionLabel)
	case config.ConnTypeMuxClient, config.ConnTypeMuxServer:
		return grpcutil.NewMultiClientConn(lifetime, fmt.Sprintf("client-conn-%s", connectionName),
			// TLS is handled by the mux connection, so tlsConfig will always be nil
			grpcutil.MakeDialOptions(nil, metrics.GetGRPCClientMetrics(directionLabel))...)
	default:
		return nil, errors.New("invalid connection type")
	}
}

func createServer(lifetime context.Context, name string, cd config.ClusterDefinition,
	directionLabel string, client closableClientConn, managedClient closableClientConn,
	nsTranslations collect.StaticBiMap[string, string], saTranslations config.SearchAttributeTranslation, logger log.Logger) (contextAwareServer, *ReplicationStreamObserver, error) {
	switch cd.Connection.ConnectionType {
	case config.ConnTypeTCP:
		// No special logic required for managedClient
		return createTCPServer(lifetime, client, name, cd, directionLabel, nsTranslations, saTranslations, logger)
	case config.ConnTypeMuxClient, config.ConnTypeMuxServer:
		observer := NewReplicationStreamObserver(logger)
		grpcServer, err := buildProxyServer(client, directionLabel, cd, observer, nsTranslations, saTranslations, logger)
		if err != nil {
			return nil, nil, err
		}
		// The Mux manager needs to update its associated client
		muxMgr, err := mux.NewGRPCMuxManager(lifetime, name, cd, managedClient.(*grpcutil.MultiClientConn), grpcServer, logger)
		if err != nil {
			return nil, nil, err
		}
		return muxMgr, observer, nil
	default:
		return nil, nil, errors.New("invalid connection type")
	}
}

func createTCPServer(lifetime context.Context, client closableClientConn, name string,
	cd config.ClusterDefinition, directionLabel string, nsTranslations collect.StaticBiMap[string, string],
	saTranslations config.SearchAttributeTranslation, logger log.Logger) (contextAwareServer, *ReplicationStreamObserver, error) {
	observer := NewReplicationStreamObserver(logger)
	listener, err := net.Listen("tcp", cd.Connection.TcpServer.ConnectionString)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid configuration for inbound server: %w", err)
	}
	grpcServer, err := buildProxyServer(client, directionLabel, cd, observer, nsTranslations, saTranslations, logger)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create inbound server: %w", err)
	}
	server := &simpleGRPCServer{
		name:     name,
		lifetime: lifetime,
		listener: listener,
		server:   grpcServer,
		logger:   logger,
	}
	return server, observer, nil
}

// buildTLSTCPClient creates a grpc.ClientConn using the provided configuration. It schedules a goroutine that closes
// the grpc.ClientConn when the provided lifetime ends.
func buildTLSTCPClient(lifetime context.Context, serverAddress string, tlsCfg encryption.TLSConfig, metricLabel string) (closableClientConn, error) {
	var parsedTLSCfg *tls.Config
	if tlsCfg.IsEnabled() {
		var err error
		parsedTLSCfg, err = encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("config error when creating tls config: %w", err)
		}
	}
	client, err := grpc.NewClient(serverAddress, grpcutil.MakeDialOptions(parsedTLSCfg, metrics.GetGRPCClientMetrics(metricLabel))...)
	if err != nil {
		return nil, fmt.Errorf("could not create inbound client: %w", err)
	}
	context.AfterFunc(lifetime, func() {
		// grpc.ClientConn must be closed, but it's not context-aware. Make sure the client closes when the lifetime ends
		_ = client.Close()
	})
	return describableClientConn{client}, nil
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
func buildProxyServer(client grpc.ClientConnInterface, directionLabel string, cd config.ClusterDefinition,
	observer *ReplicationStreamObserver, nsTranslations collect.StaticBiMap[string, string], saTranslations config.SearchAttributeTranslation, logger log.Logger) (*grpc.Server, error) {
	serverOpts, err := MakeServerOptions(logger, directionLabel, cd.Connection.TcpServer.TLSConfig, cd.ACLPolicy, nsTranslations, saTranslations)
	if err != nil {
		return nil, fmt.Errorf("could not parse server options: %w", err)
	}
	server := grpc.NewServer(serverOpts...)
	adminServiceImpl := NewAdminServiceProxyServer(fmt.Sprintf("%sAdminService", directionLabel), adminservice.NewAdminServiceClient(client),
		cd.APIOverrides, []string{directionLabel}, observer.ReportStreamValue, logger)
	var accessControl *auth.AccessControl
	if cd.ACLPolicy != nil {
		accessControl = auth.NewAccesControl(cd.ACLPolicy.AllowedNamespaces)
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
