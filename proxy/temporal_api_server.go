package proxy

import (
	"fmt"
	"net"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	TemporalAPIServer struct {
		serviceName            string
		serverConfig           config.ServerConfig
		server                 *grpc.Server
		adminHandler           adminservice.AdminServiceServer
		workflowserviceHandler workflowservice.WorkflowServiceServer
		logger                 log.Logger
	}
)

func NewTemporalAPIServer(
	serviceName string,
	serverConfig config.ServerConfig,
	adminHandler adminservice.AdminServiceServer,
	workflowserviceHandler workflowservice.WorkflowServiceServer,
	serverOptions []grpc.ServerOption,
	logger log.Logger,
) *TemporalAPIServer {
	server := grpc.NewServer(serverOptions...)
	return &TemporalAPIServer{
		serviceName:            serviceName,
		serverConfig:           serverConfig,
		server:                 server,
		adminHandler:           adminHandler,
		workflowserviceHandler: workflowserviceHandler,
		logger:                 logger,
	}
}

func (s *TemporalAPIServer) Start() error {
	adminservice.RegisterAdminServiceServer(s.server, s.adminHandler)
	workflowservice.RegisterWorkflowServiceServer(s.server, s.workflowserviceHandler)

	grpcListener, err := net.Listen("tcp", s.serverConfig.ListenAddress)
	if err != nil {
		s.logger.Fatal("Failed to start gRPC listener", tag.Error(err))
		return err
	}

	s.logger.Info(fmt.Sprintf("Starting %s with config: %v", s.serviceName, s.serverConfig))
	go func() {
		if err := s.server.Serve(grpcListener); err != nil {
			s.logger.Fatal("Failed to start proxy", tag.Error(err))
		}
	}()

	return nil
}

func (s *TemporalAPIServer) Stop() {
	s.logger.Info(fmt.Sprintf("Stopping %s", s.serviceName))
	s.server.GracefulStop()
}
