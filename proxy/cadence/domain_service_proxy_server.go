package cadence

import (
	"context"
	apiv1 "github.com/uber/cadence-idl/go/proto/api/v1"
	"go.temporal.io/server/common/log"
)

var _ apiv1.DomainAPIYARPCServer = domainServiceProxyServer{}

type domainServiceProxyServer struct {
	logger log.Logger
}

func (d domainServiceProxyServer) RegisterDomain(ctx context.Context, request *apiv1.RegisterDomainRequest) (*apiv1.RegisterDomainResponse, error) {
	//TODO implement me
	panic("implement me")
}

func (d domainServiceProxyServer) DescribeDomain(ctx context.Context, request *apiv1.DescribeDomainRequest) (*apiv1.DescribeDomainResponse, error) {
	d.logger.Info("Cadence API server: DescribeDomain called.")
	return &apiv1.DescribeDomainResponse{
		Domain: &apiv1.Domain{
			Name:        "test-domain",
			Status:      apiv1.DomainStatus_DOMAIN_STATUS_REGISTERED,
			Description: "test-domain-description",
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
