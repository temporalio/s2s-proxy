package cadence

import (
	"context"
	"github.com/gogo/protobuf/types"
	"github.com/temporalio/s2s-proxy/client"
	feclient "github.com/temporalio/s2s-proxy/client/frontend"
	"github.com/temporalio/s2s-proxy/config"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
)

var _ apiv1.DomainAPIYARPCServer = domainServiceProxyServer{}

type domainServiceProxyServer struct {
	workflowServiceClient workflowservice.WorkflowServiceClient
	logger                log.Logger
}

func NewDomainServiceProxyServer(
	clientConfig config.ProxyClientConfig,
	clientFactory client.ClientFactory,
	logger log.Logger,
) apiv1.DomainAPIYARPCServer {
	clientProvider := client.NewClientProvider(clientConfig, clientFactory, logger)

	return domainServiceProxyServer{
		workflowServiceClient: feclient.NewLazyClient(clientProvider),
		logger:                logger,
	}
}

func (d domainServiceProxyServer) RegisterDomain(ctx context.Context, request *apiv1.RegisterDomainRequest) (*apiv1.RegisterDomainResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (d domainServiceProxyServer) DescribeDomain(ctx context.Context, request *apiv1.DescribeDomainRequest) (*apiv1.DescribeDomainResponse, error) {
	d.logger.Info("Cadence API server: DescribeDomain called.")

	tReq := &workflowservice.DescribeNamespaceRequest{
		Namespace: request.GetName(),
		Id:        request.GetId(),
	}
	resp, err := d.workflowServiceClient.DescribeNamespace(ctx, tReq)
	return d.convertToCadenceResp(resp, err)
}

func (d domainServiceProxyServer) convertToCadenceResp(resp *workflowservice.DescribeNamespaceResponse, err error) (*apiv1.DescribeDomainResponse, error) {
	if err != nil {
		d.logger.Error("Cadence API server: Failed to describe domain", tag.Error(err))

		switch err.(type) {
		case *serviceerror.NotFound:
			return nil, serviceerror.NewNotFound("domain not found")
		}
		return nil, err
	}

	d.logger.Info("Cadence API server: DescribeDomain succeeded.", tag.Value(resp.NamespaceInfo.Name))
	return &apiv1.DescribeDomainResponse{
		Domain: &apiv1.Domain{
			Id:          resp.NamespaceInfo.Id,
			Name:        resp.NamespaceInfo.Name,
			Status:      apiv1.DomainStatus(resp.NamespaceInfo.GetState()),
			Description: resp.NamespaceInfo.Description,
			OwnerEmail:  resp.NamespaceInfo.OwnerEmail,
			Data:        resp.NamespaceInfo.Data,
			WorkflowExecutionRetentionPeriod: &types.Duration{
				Seconds: resp.Config.GetWorkflowExecutionRetentionTtl().GetSeconds(),
			},
			ActiveClusterName: resp.GetReplicationConfig().GetActiveClusterName(),
		},
	}, nil
}

func (d domainServiceProxyServer) ListDomains(ctx context.Context, request *apiv1.ListDomainsRequest) (*apiv1.ListDomainsResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (d domainServiceProxyServer) UpdateDomain(ctx context.Context, request *apiv1.UpdateDomainRequest) (*apiv1.UpdateDomainResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (d domainServiceProxyServer) DeprecateDomain(ctx context.Context, request *apiv1.DeprecateDomainRequest) (*apiv1.DeprecateDomainResponse, error) {
	//TODO implement me
	panic("implement me")
}
