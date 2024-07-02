package proxy

import (
	"net"

	"github.com/temporalio/s2s-proxy/client"
	"github.com/temporalio/s2s-proxy/client/rpc"
	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	proxyServer struct {
		serviceName  string
		proxyAddress string
		server       *grpc.Server
		adminHandler adminservice.AdminServiceServer
		logger       log.Logger
	}

	Proxy struct {
		config     config.Config
		rpcFactory rpc.RPCFactory

		outboundServer *proxyServer
		inboundServer  *proxyServer
	}
)

func newProxyServer(
	serviceName string,
	proxyAddress string,
	adminHandler adminservice.AdminServiceServer,
	serverOptions []grpc.ServerOption,
	logger log.Logger,
) *proxyServer {
	server := grpc.NewServer(serverOptions...)
	return &proxyServer{
		serviceName:  serviceName,
		proxyAddress: proxyAddress,
		server:       server,
		adminHandler: adminHandler,
		logger:       logger,
	}
}

func (ps *proxyServer) start() error {
	adminservice.RegisterAdminServiceServer(ps.server, ps.adminHandler)
	grpcListener, err := net.Listen("tcp", ps.proxyAddress)
	if err != nil {
		ps.logger.Fatal("Failed to start gRPC listener", tag.Error(err), common.ServiceTag(ps.serviceName), tag.Address(ps.proxyAddress))
		return err
	}

	ps.logger.Info("Created gRPC listener", common.ServiceTag(ps.serviceName), tag.Address(ps.proxyAddress))
	go func() {
		ps.logger.Info("Starting proxy", common.ServiceTag(ps.serviceName), tag.Address(ps.proxyAddress))
		if err := ps.server.Serve(grpcListener); err != nil {
			ps.logger.Fatal("Failed to start proxy", common.ServiceTag(ps.serviceName), tag.Address(ps.proxyAddress), tag.Error(err))
		}
	}()

	return nil
}

func (ps *proxyServer) stop() {
	ps.logger.Info("Stopping proxy", common.ServiceTag(ps.serviceName), tag.Address(ps.proxyAddress))
	ps.server.GracefulStop()
}

func NewProxy(
	config config.Config,
	logger log.Logger,
	rpcFactory rpc.RPCFactory,
	clientFactory client.ClientFactory,
) *Proxy {
	remoteClient := clientFactory.NewRemoteAdminClient(config.GetRemoteServerRPCAddress())
	localClient := clientFactory.NewRemoteAdminClient(config.GetLocalServerRPCAddress())
	return &Proxy{
		config: config,
		outboundServer: newProxyServer(
			"outbound-server",
			config.GetOutboundServerAddress(),
			NewAdminServiceProxyServer(config.GetRemoteServerRPCAddress(), remoteClient, logger),
			nil, // grpc server options
			logger,
		),
		inboundServer: newProxyServer(
			"inbound-server",
			config.GetInboundServerAddress(),
			NewAdminServiceProxyServer(config.GetLocalServerRPCAddress(), localClient, logger),
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
