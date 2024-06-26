package proxy

import (
	"net"

	"github.com/temporalio/temporal-proxy/config"
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
	}
)

func NewProxy(
	config config.Config,
	server *grpc.Server,
	logger log.Logger,
	grpcListener net.Listener,
) *Proxy {
	return &Proxy{
		config:       config,
		server:       server,
		logger:       logger,
		grpcListener: grpcListener,
	}
}

func (s *Proxy) Start() error {
	go func() {
		s.logger.Info("Starting proxy listener", tag.Port(s.config.GetGRPCPort()))
		if err := s.server.Serve(s.grpcListener); err != nil {
			s.logger.Fatal("Failed to start proxy listener", tag.Error(err))
		}
	}()

	return nil
}

func (s *Proxy) Stop() {
	s.logger.Info("Stopping proxy listener")
}
