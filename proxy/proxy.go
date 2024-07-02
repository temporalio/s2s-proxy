package proxy

import (
	"net"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	Proxy struct {
		config       config.Config
		server       *grpc.Server
		logger       log.Logger
		grpcListener net.Listener
		adminHandler adminservice.AdminServiceServer
	}
)

func NewProxy(
	config config.Config,
	server *grpc.Server,
	logger log.Logger,
	grpcListener net.Listener,
	adminHandler adminservice.AdminServiceServer,
) *Proxy {
	return &Proxy{
		config:       config,
		server:       server,
		logger:       logger,
		grpcListener: grpcListener,
		adminHandler: adminHandler,
	}
}

func (s *Proxy) Start() error {
	adminservice.RegisterAdminServiceServer(s.server, s.adminHandler)

	go func() {
		s.logger.Info("Starting proxy listener", tag.Port(s.config.GetListenPort()))
		if err := s.server.Serve(s.grpcListener); err != nil {
			s.logger.Fatal("Failed to start proxy listener", tag.Error(err))
		}
	}()

	return nil
}

func (s *Proxy) Stop() {
	s.logger.Info("Stopping proxy listener")
	s.server.GracefulStop()
}
