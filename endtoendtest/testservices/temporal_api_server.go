package testservices

import (
	"fmt"
	"io"
	"net"
	"time"

	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/channel"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	TemporalServerWithListen struct {
		serviceName   string
		server        *grpc.Server
		listenAddr    string
		serverStopped channel.ShutdownOnce
		logger        log.Logger
	}
)

func NewTemporalAPIServer(
	serviceName string,
	adminHandler adminservice.AdminServiceServer,
	workflowserviceHandler workflowservice.WorkflowServiceServer,
	serverOptions []grpc.ServerOption,
	listenAddr string,
	logger log.Logger,
) *TemporalServerWithListen {
	server := grpc.NewServer(serverOptions...)
	adminservice.RegisterAdminServiceServer(server, adminHandler)
	workflowservice.RegisterWorkflowServiceServer(server, workflowserviceHandler)
	return &TemporalServerWithListen{
		serverStopped: channel.NewShutdownOnce(),
		serviceName:   serviceName,
		server:        server,
		listenAddr:    listenAddr,
		logger:        logger,
	}
}

func (s *TemporalServerWithListen) Start() {
	listener, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		panic(err)
	}
	go func() {
		for !s.serverStopped.IsShutdown() {
			err := s.server.Serve(listener)
			if err == io.EOF {
				// grpc server can get EOF error if grpc server relies on client side of
				// mux connection. Given a mux connection from node A (mux client) to node B (mux server),
				// and start a grpc server on A using mux client. If node B (mux server) closed the connection,
				// grpc server on A can get an EOF client connection error from underlying mux connection.
				// It should not happen if grpc server is based on mux server or normal TCP connection.
				s.logger.Info("grpc server received EOF! Connection is closing")
			} else if err != nil {
				s.logger.Error("grpc server fatal error ", tag.Error(err))
			}
			time.Sleep(1 * time.Second)
		}
		_ = listener.Close()
	}()
}

func (s *TemporalServerWithListen) Stop() {
	s.logger.Info(fmt.Sprintf("Stopping %s", s.serviceName))
	s.serverStopped.Shutdown()
	s.server.GracefulStop()
	s.logger.Info(fmt.Sprintf("Stopped %s", s.serviceName))
}

func (s *TemporalServerWithListen) ForceStop() {
	s.logger.Info(fmt.Sprintf("Stopping %s forcefully", s.serviceName))
	s.serverStopped.Shutdown()
	s.server.Stop()
}
