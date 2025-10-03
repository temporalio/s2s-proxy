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
	simpleGRPCServer struct {
		name     string
		lifetime context.Context
		listener net.Listener
		server   *grpc.Server
		logger   log.Logger
	}

	describableClientConn struct {
		*grpc.ClientConn
	}

	// ClusterConnection contains a bidirectional connection between a local Temporal server and a remote.
	ClusterConnection struct {
		// When lifetime closes, all clients and servers in ClusterConnection will stop.
		lifetime context.Context
		// outboundServer receives local connections and makes calls using outboundClient
		outboundServer contextAwareServer
		// outboundClient is connected to a remote Temporal server somewhere. It has both
		outboundClient closableClientConn
		inboundServer  contextAwareServer
		inboundClient  closableClientConn
		logger         log.Logger
	}
	contextAwareServer interface {
		Start()
		IsUsable() bool
		Describe() string
	}
	closableClientConn interface {
		grpc.ClientConnInterface
		Close() error
		Describe() string
	}
)

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
	cc.inboundClient, cc.inboundServer, err = buildSimpleServerArc(lifetime, true, outbound.Server.ExternalAddress,
		inbound, "inbound", namespaceNameTranslation, searchAttributeTranslation, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build inbound config %s: %w", inbound.Name, err)
	}
	cc.outboundClient, cc.outboundServer, err = buildSimpleServerArc(lifetime, false, "",
		outbound, "outbound", namespaceNameTranslation, searchAttributeTranslation, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to build outbound config %s: %w", outbound.Name, err)
	}
	return cc, nil
}

func buildSimpleServerArc(lifetime context.Context, isInbound bool, overrideExternalAddress string,
	proxyCfg config.ProxyConfig, directionLabel string, namespaceNameTranslation config.NameTranslationConfig,
	searchAttributeTranslation config.SATranslationConfig, logger log.Logger) (closableClientConn, contextAwareServer, error) {
	client, err := buildTLSTCPClient(lifetime, proxyCfg.Client.ServerAddress, proxyCfg.Client.TLS, directionLabel)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create %s tls client: %w", directionLabel, err)
	}

	listener, err := net.Listen("tcp", proxyCfg.Server.ListenAddress)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid configuration for inbound server: %w", err)
	}
	serverCfg, err := buildProxyServer(client, isInbound, proxyCfg, overrideExternalAddress,
		namespaceNameTranslation, searchAttributeTranslation, logger)
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

func buildTLSTCPClient(lifetime context.Context, serverAddress string, tlsCfg encryption.ClientTLSConfig, directionLabel string) (closableClientConn, error) {
	var parsedTLSCfg *tls.Config
	if tlsCfg.IsEnabled() {
		var err error
		parsedTLSCfg, err = encryption.GetClientTLSConfig(tlsCfg)
		if err != nil {
			return nil, fmt.Errorf("config error when creating tls config: %w", err)
		}
	}
	client, err := grpc.NewClient(serverAddress, grpcutil.MakeDialOptions(parsedTLSCfg, metrics.GetStandardGRPCClientInterceptor(directionLabel))...)
	if err != nil {
		return nil, fmt.Errorf("could not create inbound client: %w", err)
	}
	context.AfterFunc(lifetime, func() {
		// clientConns must be closed, but they are not context-aware
		_ = client.Close()
	})
	return describableClientConn{client}, nil
}

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
	cc.outboundClient, err = grpcutil.NewMultiClientConn(fmt.Sprintf("client-conn-%s", muxConfig.Name), grpcutil.MakeDialOptions(nil, metrics.GetStandardGRPCClientInterceptor("outbound"))...)
	if err != nil {
		return nil, fmt.Errorf("failed to create multi client connection for mux: %w", err)
	}
	context.AfterFunc(cc.lifetime, func() {
		// clientConns must be closed, but they are not context-aware
		_ = cc.outboundClient.Close()
	})
	cc.inboundClient, err = buildTLSTCPClient(lifetime, inbound.Client.ServerAddress, inbound.Client.TLS, "inbound")
	if err != nil {
		return nil, fmt.Errorf("failed to create inbound client: %w", err)
	}

	// inbound server
	inboundServerCfg, err := buildProxyServer(cc.inboundClient, true, inbound, outbound.Server.ExternalAddress,
		namespaceNameTranslation, searchAttributeTranslation, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create inbound server for config %s: %w", inbound.Name, err)
	}

	cc.inboundServer, err = mux.NewGRPCMuxManager(lifetime, muxConfig, cc.outboundClient.(*grpcutil.MultiClientConn), inboundServerCfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create GRPC mux manager for config named %s: %w", muxConfig.Name, err)
	}

	// outbound server
	outboundListener, err := net.Listen("tcp", outbound.Server.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on outbound server address %s: %w", outbound.Server.ListenAddress, err)
	}
	outboundServerCfg, err := buildProxyServer(cc.outboundClient, false, outbound, "", namespaceNameTranslation, searchAttributeTranslation, logger)
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
	c.outboundServer.Start()
}
func (c *ClusterConnection) RemoteUsable() bool {
	return c.inboundServer.IsUsable()
}
func (c *ClusterConnection) LocalUsable() bool {
	return c.outboundServer.IsUsable()
}
func (c *ClusterConnection) Describe() string {
	return fmt.Sprintf("[ClusterConnection connects outbound server %s to outbound client %s, inbound server %s to inbound client %s]",
		c.outboundServer.Describe(), c.outboundClient.Describe(), c.inboundServer.Describe(), c.inboundClient.Describe())
}

func buildProxyServer(client grpc.ClientConnInterface, isInbound bool, serverCfg config.ProxyConfig, overrideExternalAddress string,
	namespaceTranslation config.NameTranslationConfig, searchAttrTranslation config.SATranslationConfig, logger log.Logger) (*grpc.Server, error) {
	directionLabel := "inbound"
	if isInbound {
		directionLabel = "outbound"
	}
	serverOpts, err := MakeServerOptions(logger, isInbound, serverCfg.Server.TLS, serverCfg.ACLPolicy, namespaceTranslation, searchAttrTranslation)
	if err != nil {
		return nil, fmt.Errorf("could not parse server options: %w", err)
	}
	server := grpc.NewServer(serverOpts...)
	adminServiceImpl := NewAdminServiceProxyServer(fmt.Sprintf("%sAdminService", directionLabel), adminservice.NewAdminServiceClient(client),
		overrideExternalAddress, serverCfg.APIOverrides, []string{directionLabel}, logger)
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
func (s *simpleGRPCServer) IsUsable() bool {
	return true
}
func (s *simpleGRPCServer) Describe() string {
	return fmt.Sprintf("[simpleGRPCServer %s listening on %s. lifetime.Err: %e]", s.name, s.listener.Addr(), s.lifetime.Err())
}

func (d describableClientConn) Describe() string {
	return fmt.Sprintf("[grpc.ClientConn %s, state=%s]", d.Target(), d.GetState().String())
}
