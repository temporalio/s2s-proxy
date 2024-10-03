package interceptor

import (
	"context"
	"strings"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	NamespaceNameTranslator struct {
		logger                      log.Logger
		requestNameMapping          map[string]string
		responseNameMapping         map[string]string
	}
)

func NewNamespaceNameTranslator(
	logger log.Logger,
	cfg config.ProxyConfig,
	isInbound bool,
) *NamespaceNameTranslator {
	requestNameMapping := map[string]string{}
	responseNameMapping := map[string]string{}
	for _, tr := range cfg.NamespaceNameTranslation.Mappings {
		if isInbound {
			// For inbound listener,
			//   - incoming requests from remote server are modifed to match local server
			//   - outgoing responses to local server are modified to match remote server
			requestNameMapping[tr.RemoteName] = tr.LocalName
			responseNameMapping[tr.LocalName] = tr.RemoteName
		} else {
			// For outbound listener,
			//   - incoming requests from local server are modifed to match remote server
			//   - outgoing responses to remote server are modified to match local server
			requestNameMapping[tr.LocalName] = tr.RemoteName
			responseNameMapping[tr.RemoteName] = tr.LocalName
		}
	}

	return &NamespaceNameTranslator{
		logger:                      logger,
		requestNameMapping:          requestNameMapping,
		responseNameMapping:         responseNameMapping,
	}
}

var _ grpc.UnaryServerInterceptor = (*NamespaceNameTranslator)(nil).Intercept

func (i *NamespaceNameTranslator) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if len(i.requestNameMapping) == 0 {
		return handler(ctx, req)
	}

	if strings.HasPrefix(info.FullMethod, api.WorkflowServicePrefix) ||
		strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {

		methodName := api.MethodName(info.FullMethod)
		i.logger.Debug("intercepted request", tag.NewStringTag("method", methodName))

		// Translate namespace name in request.
		changed, trErr := translateNamespace(req, i.requestNameMapping)
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Request", req)

		resp, err := handler(ctx, req)

		// Translate namespace name in response.
		changed, trErr = translateNamespace(resp, i.responseNameMapping)
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Response", resp)
		return resp, err
	} else {
		return handler(ctx, req)
	}
}

func logTranslateNamespaceResult(logger log.Logger, changed bool, err error, methodName string, obj any) {
	logger = log.With(
		logger,
		tag.NewStringTag("method", methodName),
		tag.NewAnyTag("obj", obj),
	)
	if err != nil {
		logger.Error("namespace translation error", tag.NewErrorTag(err))
	} else if changed {
		logger.Debug("namespace translation applied")
	} else {
		logger.Debug("namespace translation not applied")
	}
}
