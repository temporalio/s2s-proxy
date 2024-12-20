package cadence

import (
	"github.com/temporalio/s2s-proxy/client"
	adminclient "github.com/temporalio/s2s-proxy/client/admin"
	feclient "github.com/temporalio/s2s-proxy/client/frontend"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/transport"
	adminv1 "github.com/uber/cadence-idl/go/proto/admin/v1"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.uber.org/yarpc"
	"go.uber.org/yarpc/transport/grpc"
	"net"
)

type (
	CadenceAPIServer struct {
		serviceName  string
		GRPCAddress  string
		clientConfig config.ProxyClientConfig
		transManager *transport.TransportManager
		dispatcher   *yarpc.Dispatcher
		logger       log.Logger

		workflowProxy   apiv1.WorkflowAPIYARPCServer
		domainProxy     apiv1.DomainAPIYARPCServer
		workerProxy     apiv1.WorkerAPIYARPCServer
		visibilityProxy apiv1.VisibilityAPIYARPCServer
		metaProxy       apiv1.MetaAPIYARPCServer
		adminProxy      adminv1.AdminAPIYARPCServer
	}
)

func NewCadenceAPIServer(
	logger log.Logger,
	clientConfig config.ProxyClientConfig,
	clientFactory client.ClientFactory,
) *CadenceAPIServer {
	clientProvider := client.NewClientProvider(clientConfig, clientFactory, logger)
	workflowServiceClient := feclient.NewLazyClient(clientProvider)
	adminServiceClient := adminclient.NewLazyClient(clientProvider)
	return &CadenceAPIServer{
		serviceName:     "cadence-frontend",
		GRPCAddress:     "localhost:7733",
		logger:          logger,
		domainProxy:     NewDomainServiceProxyServer(logger, workflowServiceClient),
		workflowProxy:   NewWorkflowServiceProxyServer(logger, workflowServiceClient),
		workerProxy:     NewWorkerServiceProxyServer(logger, workflowServiceClient),
		visibilityProxy: NewVisibilityServiceProxyServer(logger, workflowServiceClient),
		metaProxy:       NewMetaServiceProxyServer(logger, workflowServiceClient),
		adminProxy:      NewAdminServiceProxyServer(logger, adminServiceClient),
	}
}

func (s *CadenceAPIServer) Start() {
	s.logger.Info("Starting Cadence API server", tag.Address(s.GRPCAddress))

	inbounds := yarpc.Inbounds{}
	grpcTransport := grpc.NewTransport()
	if len(s.GRPCAddress) > 0 {
		listener, err := net.Listen("tcp", s.GRPCAddress)
		if err != nil {
			s.logger.Fatal("Failed to listen on GRPC port", tag.Error(err))
		}

		//var inboundOptions []grpc.InboundOption
		//if p.InboundTLS != nil {
		//	inboundOptions = append(inboundOptions, grpc.InboundCredentials(credentials.NewTLS(p.InboundTLS)))
		//}

		inbounds = append(inbounds, grpcTransport.NewInbound(listener))
		s.logger.Info("Listening for GRPC requests", tag.Address(s.GRPCAddress))
	}

	s.dispatcher = yarpc.NewDispatcher(yarpc.Config{
		Name:     s.serviceName,
		Inbounds: inbounds,
	})

	s.dispatcher.Register(apiv1.BuildDomainAPIYARPCProcedures(s.domainProxy))
	s.dispatcher.Register(apiv1.BuildWorkflowAPIYARPCProcedures(s.workflowProxy))
	s.dispatcher.Register(apiv1.BuildWorkerAPIYARPCProcedures(s.workerProxy))
	s.dispatcher.Register(apiv1.BuildVisibilityAPIYARPCProcedures(s.visibilityProxy))
	s.dispatcher.Register(apiv1.BuildMetaAPIYARPCProcedures(s.metaProxy))
	s.dispatcher.Register(adminv1.BuildAdminAPIYARPCProcedures(s.adminProxy))

	err := s.dispatcher.Start()
	if err != nil {
		s.logger.Fatal("Failed to start dispatcher", tag.Error(err))
	}
}

func (s *CadenceAPIServer) Stop() {
	s.logger.Info("Stopping Cadence API server")
	s.dispatcher.Stop()
}
