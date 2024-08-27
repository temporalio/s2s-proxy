package proxy

import (
	"net"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/interceptor"
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

	GrpcServerOptions struct {
		Options           []grpc.ServerOption
		UnaryInterceptors []grpc.UnaryServerInterceptor
	}
)

func GrpcServerOptionsProvider(
	logger log.Logger,
	namespaceTranslatorInterceptor *interceptor.NamespaceTranslator,
) GrpcServerOptions {
	return GrpcServerOptions{
		UnaryInterceptors: []grpc.UnaryServerInterceptor{
			namespaceTranslatorInterceptor.Intercept,
		},
	}
}

func NewTemporalAPIServer(
	serviceName string,
	serverConfig config.ServerConfig,
	adminHandler adminservice.AdminServiceServer,
	grpcServerOptions GrpcServerOptions,
	logger log.Logger,
) *TemporalAPIServer {
	opts := []grpc.ServerOption{}
	opts = append(opts, grpcServerOptions.Options...)
	opts = append(opts, grpc.ChainUnaryInterceptor(grpcServerOptions.UnaryInterceptors...))
	server := grpc.NewServer(opts...)

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
