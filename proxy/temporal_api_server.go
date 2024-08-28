package proxy

import (
	"encoding/json"
	"fmt"
	"net"

	"github.com/temporalio/s2s-proxy/common"
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
	logger = log.With(logger, common.ServiceTag(serviceName), tag.Address(serverConfig.ListenAddress))
	if data, err := json.Marshal(serverConfig); err == nil {
		logger.Info(fmt.Sprintf("TemporalAPIServer Config: %s", string(data)))
	} else {
		logger.Error(fmt.Sprintf("TemporalAPIServer: failed to marshal config: %v", err))
		return nil
	}

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

func (ps *TemporalAPIServer) Start() error {
	adminservice.RegisterAdminServiceServer(ps.server, ps.adminHandler)
	workflowservice.RegisterWorkflowServiceServer(ps.server, ps.workflowserviceHandler)

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
