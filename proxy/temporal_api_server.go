package proxy

import (
	"fmt"
	"io"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport"
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
		serverTransport        transport.ServerTransport
		logger                 log.Logger
	}
)

func NewTemporalAPIServer(
	serviceName string,
	serverConfig config.ServerConfig,
	adminHandler adminservice.AdminServiceServer,
	workflowserviceHandler workflowservice.WorkflowServiceServer,
	serverOptions []grpc.ServerOption,
	serverTransport transport.ServerTransport,
	logger log.Logger,
) *TemporalAPIServer {
	server := grpc.NewServer(serverOptions...)
	return &TemporalAPIServer{
		serviceName:            serviceName,
		serverConfig:           serverConfig,
		server:                 server,
		adminHandler:           adminHandler,
		workflowserviceHandler: workflowserviceHandler,
		serverTransport:        serverTransport,
		logger:                 logger,
	}
}

func (s *TemporalAPIServer) Start() error {
	adminservice.RegisterAdminServiceServer(s.server, s.adminHandler)
	workflowservice.RegisterWorkflowServiceServer(s.server, s.workflowserviceHandler)

	s.logger.Info(fmt.Sprintf("Starting %s with config: %v", s.serviceName, s.serverConfig))
	go func() {
		if err := s.serverTransport.Serve(s.server); err != nil {
			if err == io.EOF {
				// grpc server can get EOF error if grpc server relies on client side of
				// mux connection. Given a mux connection from node A (mux client) to node B (mux server),
				// we start a grpc server on A using mux client. If node B (mux server) closed the connection,
				// grpc server on A can get an EOF client connection error from underlying mux connection.
				// It should not happen if grpc server is based on mux server or normal TCP connection.
				s.logger.Warn("grpc server received EOF error")
			} else {
				s.logger.Fatal("grpc server fatal error ", tag.Error(err))
			}
		}
	}()

	return nil
}

func (s *TemporalAPIServer) Stop() {
	s.logger.Info(fmt.Sprintf("Stopping %s", s.serviceName))
	s.server.GracefulStop()
}
