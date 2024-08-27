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
		serviceName  string
		serverConfig config.ServerConfig
		server       *grpc.Server
		adminHandler adminservice.AdminServiceServer
		logger       log.Logger
	}
)

func NewTemporalAPIServer(
	serviceName string,
	serverConfig config.ServerConfig,
	adminHandler adminservice.AdminServiceServer,
	serverOptions []grpc.ServerOption,
	logger log.Logger,
) *TemporalAPIServer {
<<<<<<< HEAD
	server := grpc.NewServer(serverOptions...)
=======
	// TODO: Add TLS option

	opts := []grpc.ServerOption{}
	opts = append(opts, grpcServerOptions.Options...)
	opts = append(opts, grpc.ChainUnaryInterceptor(grpcServerOptions.UnaryInterceptors...))
	server := grpc.NewServer(opts...)

>>>>>>> d21446d (Update code to support server-side TLS)
	return &TemporalAPIServer{
		serviceName:  serviceName,
		serverConfig: serverConfig,
		server:       server,
		adminHandler: adminHandler,
		logger:       log.With(logger, common.ServiceTag(serviceName), tag.Address(serverConfig.ListenAddress)),
	}
}

func (ps *TemporalAPIServer) Start() error {
	adminservice.RegisterAdminServiceServer(ps.server, ps.adminHandler)
	grpcListener, err := net.Listen("tcp", ps.serverConfig.ListenAddress)
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
