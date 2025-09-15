package proxy

import (
	"fmt"
	"io"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/metrics"
	"github.com/temporalio/s2s-proxy/transport"
)

type (
	TemporalAPIServer struct {
		serviceName            string
		serverConfig           config.ProxyServerConfig
		server                 *grpc.Server
		adminHandler           adminservice.AdminServiceServer
		workflowserviceHandler workflowservice.WorkflowServiceServer
		serverTransport        transport.ServerTransport
		logger                 log.Logger
	}
)

func NewTemporalAPIServer(
	serviceName string,
	serverConfig config.ProxyServerConfig,
	adminHandler adminservice.AdminServiceServer,
	workflowserviceHandler workflowservice.WorkflowServiceServer,
	serverOptions []grpc.ServerOption,
	serverTransport transport.ServerTransport,
	logger log.Logger,
) *TemporalAPIServer {
	server := grpc.NewServer(serverOptions...)
	adminservice.RegisterAdminServiceServer(server, adminHandler)
	workflowservice.RegisterWorkflowServiceServer(server, workflowserviceHandler)
	metrics.GRPCServerStarted.WithLabelValues(serviceName)
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

func (s *TemporalAPIServer) Start() {
	go func() {
		for !s.serverTransport.IsClosed() {
			metrics.GRPCServerStarted.WithLabelValues(s.serviceName).Inc()
			if err := s.serverTransport.Serve(s.server); err != nil {
				if err == io.EOF {
					// grpc server can get EOF error if grpc server relies on client side of
					// mux connection. Given a mux connection from node A (mux client) to node B (mux server),
					// and start a grpc server on A using mux client. If node B (mux server) closed the connection,
					// grpc server on A can get an EOF client connection error from underlying mux connection.
					// It should not happen if grpc server is based on mux server or normal TCP connection.
					s.logger.Info("grpc server received EOF! Connection is closing")
					metrics.GRPCServerError.WithLabelValues(s.serviceName, "eof").Inc()
				} else {
					s.logger.Error("grpc server fatal error ", tag.Error(err))
					metrics.GRPCServerError.WithLabelValues(s.serviceName, "unknown").Inc()
				}
			}
		}
	}()
}

func (s *TemporalAPIServer) Stop() {
	s.logger.Info(fmt.Sprintf("Stopping %s", s.serviceName))
	s.server.GracefulStop()
	s.logger.Info(fmt.Sprintf("Stopped %s", s.serviceName))
}

func (s *TemporalAPIServer) ForceStop() {
	s.logger.Info(fmt.Sprintf("Stopping %s forcefully", s.serviceName))
	s.server.Stop()
}
