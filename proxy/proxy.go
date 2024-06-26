package proxy

import (
	"net"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	Proxy struct {
		server       *grpc.Server
		logger       log.Logger
		grpcListener net.Listener
	}
)

func NewProxy(
	server *grpc.Server,
	logger log.Logger,
	grpcListener net.Listener,
) *Proxy {
	return &Proxy{
		server:       server,
		logger:       logger,
		grpcListener: grpcListener,
	}
}

func (s *Proxy) Start() error {
	go func() {
		s.logger.Info("Starting proxy listener")
		if err := s.server.Serve(s.grpcListener); err != nil {
			s.logger.Fatal("Failed to start proxy listener", tag.Error(err))
		}
	}()

	return nil
}

func (s *Proxy) Stop() {
	s.logger.Info("Stopping proxy listener")
}
