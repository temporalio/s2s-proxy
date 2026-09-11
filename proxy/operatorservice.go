package proxy

import (
	"context"

	"go.temporal.io/api/operatorservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/logging"
)

type operatorServiceProxyServer struct {
	operatorservice.UnimplementedOperatorServiceServer
	operatorServiceClient operatorservice.OperatorServiceClient
	logger                log.Logger
	metricLabelValues     []string
}

// NewOperatorServiceProxyServer creates an OperatorServiceServer suitable for registering with a gRPC Server.
// Requests are forwarded to the passed in OperatorService client.
func NewOperatorServiceProxyServer(
	serviceName string,
	operatorServiceClient operatorservice.OperatorServiceClient,
	metricLabelValues []string,
	logProvider logging.LoggerProvider,
) operatorservice.OperatorServiceServer {
	return &operatorServiceProxyServer{
		operatorServiceClient: operatorServiceClient,
		logger:                log.With(logProvider.Get(logging.OperatorService), common.ServiceTag(serviceName)),
		metricLabelValues:     metricLabelValues,
	}
}

func (s *operatorServiceProxyServer) AddSearchAttributes(ctx context.Context, in0 *operatorservice.AddSearchAttributesRequest) (*operatorservice.AddSearchAttributesResponse, error) {
	return s.operatorServiceClient.AddSearchAttributes(ctx, in0)
}

func (s *operatorServiceProxyServer) RemoveSearchAttributes(ctx context.Context, in0 *operatorservice.RemoveSearchAttributesRequest) (*operatorservice.RemoveSearchAttributesResponse, error) {
	return s.operatorServiceClient.RemoveSearchAttributes(ctx, in0)
}

func (s *operatorServiceProxyServer) ListSearchAttributes(ctx context.Context, in0 *operatorservice.ListSearchAttributesRequest) (*operatorservice.ListSearchAttributesResponse, error) {
	return s.operatorServiceClient.ListSearchAttributes(ctx, in0)
}

func (s *operatorServiceProxyServer) DeleteNamespace(ctx context.Context, in0 *operatorservice.DeleteNamespaceRequest) (*operatorservice.DeleteNamespaceResponse, error) {
	return s.operatorServiceClient.DeleteNamespace(ctx, in0)
}

func (s *operatorServiceProxyServer) AddOrUpdateRemoteCluster(ctx context.Context, in0 *operatorservice.AddOrUpdateRemoteClusterRequest) (*operatorservice.AddOrUpdateRemoteClusterResponse, error) {
	s.logger.Info("Received AddOrUpdateRemoteCluster",
		tag.Address(in0.GetFrontendAddress()),
		tag.NewBoolTag("Enabled", in0.GetEnableRemoteClusterConnection()),
		tag.NewStringsTag("configTags", s.metricLabelValues))
	return s.operatorServiceClient.AddOrUpdateRemoteCluster(ctx, in0)
}

func (s *operatorServiceProxyServer) RemoveRemoteCluster(ctx context.Context, in0 *operatorservice.RemoveRemoteClusterRequest) (*operatorservice.RemoveRemoteClusterResponse, error) {
	s.logger.Info("Received RemoveRemoteCluster",
		tag.NewStringTag("ClusterName", in0.GetClusterName()),
		tag.NewStringsTag("configTags", s.metricLabelValues))
	return s.operatorServiceClient.RemoveRemoteCluster(ctx, in0)
}

func (s *operatorServiceProxyServer) ListClusters(ctx context.Context, in0 *operatorservice.ListClustersRequest) (*operatorservice.ListClustersResponse, error) {
	return s.operatorServiceClient.ListClusters(ctx, in0)
}

func (s *operatorServiceProxyServer) GetNexusEndpoint(ctx context.Context, in0 *operatorservice.GetNexusEndpointRequest) (*operatorservice.GetNexusEndpointResponse, error) {
	return s.operatorServiceClient.GetNexusEndpoint(ctx, in0)
}

func (s *operatorServiceProxyServer) CreateNexusEndpoint(ctx context.Context, in0 *operatorservice.CreateNexusEndpointRequest) (*operatorservice.CreateNexusEndpointResponse, error) {
	return s.operatorServiceClient.CreateNexusEndpoint(ctx, in0)
}

func (s *operatorServiceProxyServer) UpdateNexusEndpoint(ctx context.Context, in0 *operatorservice.UpdateNexusEndpointRequest) (*operatorservice.UpdateNexusEndpointResponse, error) {
	return s.operatorServiceClient.UpdateNexusEndpoint(ctx, in0)
}

func (s *operatorServiceProxyServer) DeleteNexusEndpoint(ctx context.Context, in0 *operatorservice.DeleteNexusEndpointRequest) (*operatorservice.DeleteNexusEndpointResponse, error) {
	return s.operatorServiceClient.DeleteNexusEndpoint(ctx, in0)
}

func (s *operatorServiceProxyServer) ListNexusEndpoints(ctx context.Context, in0 *operatorservice.ListNexusEndpointsRequest) (*operatorservice.ListNexusEndpointsResponse, error) {
	return s.operatorServiceClient.ListNexusEndpoints(ctx, in0)
}
