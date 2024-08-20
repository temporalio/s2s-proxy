package proxy

import (
	"net"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	proxyServer struct {
		serviceName   string
		serverAddress string
		server        *grpc.Server
		adminHandler  adminservice.AdminServiceServer
		logger        log.Logger
	}

	Proxy struct {
		config         config.Config
		outboundServer *proxyServer
		inboundServer  *proxyServer
	}
)

func newProxyServer(
	serviceName string,
	serverAddress string,
	adminHandler adminservice.AdminServiceServer,
	serverOptions []grpc.ServerOption,
	logger log.Logger,
) *proxyServer {
	server := grpc.NewServer(serverOptions...)
	return &proxyServer{
		serviceName:   serviceName,
		serverAddress: serverAddress,
		server:        server,
		adminHandler:  adminHandler,
		logger:        log.With(logger, common.ServiceTag(serviceName), tag.Address(serverAddress)),
	}
}

func (ps *proxyServer) start() error {
	adminservice.RegisterAdminServiceServer(ps.server, ps.adminHandler)
	grpcListener, err := net.Listen("tcp", ps.serverAddress)
	if err != nil {
		ps.logger.Fatal("Failed to start gRPC listener", tag.Error(err))
		return err
	}

	ps.logger.Info("Created gRPC listener")
	go func() {
		ps.logger.Info("Starting proxy")
		if err := ps.server.Serve(grpcListener); err != nil {
			ps.logger.Fatal("Failed to start proxy", tag.Error(err))
		}
	}()

	return nil
}

func (ps *proxyServer) stop() {
	ps.logger.Info("Stopping proxy")
	ps.server.GracefulStop()
}

func NewProxy(
	config config.Config,
	logger log.Logger,
	clientFactory client.ClientFactory,
) *Proxy {
	remoteClient := clientFactory.NewRemoteAdminClient(config.GetRemoteServerRPCAddress())
	localClient := clientFactory.NewRemoteAdminClient(config.GetLocalServerRPCAddress())

	// Proxy consists of two grpc servers: inbound and outbound. The flow looks like the following:
	//    local server -> proxy(outbound) -> remote server
	//    local server <- proxy(inbound) <- remote server
	//
	// Here a remote server can be another proxy as well.
	//    server-a <-> proxy-a <-> proxy-b <-> server-b

	return &Proxy{
		config: config,
		outboundServer: newProxyServer(
			"outbound-server",
			config.GetOutboundServerAddress(),
			NewAdminServiceProxyServer(
				"outbound-server",
				config.GetOutboundServerAddress(),
				config.GetRemoteServerRPCAddress(),
				remoteClient,
				logger,
			),
			nil, // grpc server options
			logger,
		),
		inboundServer: newProxyServer(
			"inbound-server",
			config.GetInboundServerAddress(),
			NewAdminServiceProxyServer(
				"inbound-server",
				config.GetInboundServerAddress(),
				config.GetLocalServerRPCAddress(),
				localClient,
				logger,
			),
			nil, // grpc server options
			logger,
		),
	}
}

func (s *Proxy) Start() error {
	if err := s.outboundServer.start(); err != nil {
		return err
	}

	if err := s.inboundServer.start(); err != nil {
		return err
	}

	return nil
}

func (s *Proxy) Stop() {
	s.inboundServer.stop()
	s.outboundServer.stop()
}
