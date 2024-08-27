package interceptor

import (
	"context"
	"strings"

	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	NamespaceTranslator struct {
		logger log.Logger
	}
)

func NewNamespaceTranslator(
	logger log.Logger,
) *NamespaceTranslator {
	return &NamespaceTranslator{
		logger: logger,
	}
}

var _ grpc.UnaryServerInterceptor = (*NamespaceTranslator)(nil).Intercept

func (i *NamespaceTranslator) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (resp any, retError error) {
	i.logger.Debug("intercepted request",
		tag.NewStringTag("method", info.FullMethod),
	)

	if strings.HasPrefix(info.FullMethod, api.WorkflowServicePrefix) {
		// TODO: Add namespace name translation for workflowservice methods.
		// TODO: Implement workflowservice methods in proxy.
		return handler(ctx, req)
	} else if strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {
		// TODO: Modify namespace sync message.
		return handler(ctx, req)
	} else {
		return handler(ctx, req)
	}
}
