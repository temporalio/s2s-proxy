package proxy

import (
	"github.com/temporalio/s2s-proxy/common"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	TemporalAPIServer struct {
		serviceName   string
		serverAddress string
		server        *grpc.Server
		adminHandler  adminservice.AdminServiceServer
		logger        log.Logger
	}
)

func NewTemporalAPIServer(
	serviceName string,
	serverAddress string,
	adminHandler adminservice.AdminServiceServer,
	serverOptions []grpc.ServerOption,
	logger log.Logger,
) *TemporalAPIServer {
	server := grpc.NewServer(serverOptions...)
	return &TemporalAPIServer{
		serviceName:   serviceName,
		serverAddress: serverAddress,
		server:        server,
		adminHandler:  adminHandler,
		logger:        log.With(logger, common.ServiceTag(serviceName), tag.Address(serverAddress)),
	}
}
