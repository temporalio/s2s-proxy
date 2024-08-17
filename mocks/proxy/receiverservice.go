package proxy

import (
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
)

type (
	MockReceiverService struct {
		adminClient adminservice.AdminServiceClient
		serviceName string
		logger      log.Logger
	}
)

func NewMockReceiverService(
	adminClient adminservice.AdminServiceClient,
	serviceName string,
	logger log.Logger,
) *MockReceiverService {
	return &MockReceiverService{
		adminClient: adminClient,
		serviceName: serviceName,
		logger:      logger,
	}
}
