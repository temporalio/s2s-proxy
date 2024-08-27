package proxy

import (
	"net"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	TemporalAPIServer struct {
		proxyConfig  config.ProxyConfig
		server       *grpc.Server
		adminHandler adminservice.AdminServiceServer
		logger       log.Logger
	}
)

func NewTemporalAPIServer(
	proxyConfig config.ProxyConfig,
	adminHandler adminservice.AdminServiceServer,
	serverOptions []grpc.ServerOption,
	logger log.Logger,
) *TemporalAPIServer {
	server := grpc.NewServer(serverOptions...)
	return &TemporalAPIServer{
		proxyConfig:  proxyConfig,
		server:       server,
		adminHandler: adminHandler,
		logger:       log.With(logger, common.ServiceTag(proxyConfig.Name), tag.Address(proxyConfig.Server.ListenAddress)),
	}
}

func (ps *TemporalAPIServer) Start() error {
	adminservice.RegisterAdminServiceServer(ps.server, ps.adminHandler)
	grpcListener, err := net.Listen("tcp", ps.proxyConfig.Server.ListenAddress)
	if err != nil {
		ps.logger.Fatal("Failed to start gRPC listener", tag.Error(err))
		return err
	}

	ps.logger.Info("Created gRPC listener")
	go func() {
		ps.logger.Info("Starting proxy server")
		if err := ps.server.Serve(grpcListener); err != nil {
			ps.logger.Fatal("Failed to start proxy", tag.Error(err))
		}
	}()

	return nil
}

func (ps *TemporalAPIServer) Stop() {
	ps.logger.Info("Stopping proxy server")
	ps.server.GracefulStop()
}
