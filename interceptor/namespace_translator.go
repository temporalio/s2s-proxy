package interceptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	NamespaceNameTranslator struct {
		logger              log.Logger
		requestNameMapping  map[string]string
		responseNameMapping map[string]string
	}
)

func NewNamespaceNameTranslator(
	logger log.Logger,
	cfg config.ProxyConfig,
	isInbound bool,
	nameTranslations config.NamespaceNameTranslationConfig,
) *NamespaceNameTranslator {
	requestNameMapping := map[string]string{}
	responseNameMapping := map[string]string{}
	for _, tr := range nameTranslations.Mappings {
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
		logger:              logger,
		requestNameMapping:  requestNameMapping,
		responseNameMapping: responseNameMapping,
	}
}

var _ grpc.UnaryServerInterceptor = (*NamespaceNameTranslator)(nil).Intercept
var _ grpc.StreamServerInterceptor = (*NamespaceNameTranslator)(nil).InterceptStream

func createNameTranslator(mapping map[string]string) matcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}

func (i *NamespaceNameTranslator) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if common.IsRequestTranslationDisabled(ctx) {
		return handler(ctx, req)
	}

	if len(i.requestNameMapping) == 0 {
		return handler(ctx, req)
	}

	if strings.HasPrefix(info.FullMethod, api.WorkflowServicePrefix) ||
		strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {

		methodName := api.MethodName(info.FullMethod)
		i.logger.Debug("intercepted request", tag.NewStringTag("method", methodName))

		// Translate namespace name in request.
		changed, trErr := visitNamespace(req, createNameTranslator(i.requestNameMapping))
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Request", req)

		resp, err := handler(ctx, req)

		// Translate namespace name in response.
		changed, trErr = visitNamespace(resp, createNameTranslator(i.responseNameMapping))
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Response", resp)
		return resp, err
	} else {
		return handler(ctx, req)
	}
}

func (i *NamespaceNameTranslator) InterceptStream(
	srv any,
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	i.logger.Debug("InterceptStream",
		tag.NewAnyTag("method", info.FullMethod),
		tag.NewAnyTag("requestMap", i.requestNameMapping),
		tag.NewAnyTag("responseMap", i.responseNameMapping),
	)
	err := handler(srv, newStreamTranslator(
		ss,
		i.logger,
		i.requestNameMapping,
		i.responseNameMapping,
	))
	if err != nil {
		i.logger.Error("grpc handler with error: %v", tag.Error(err))
	}
	return err
}

type streamTranslator struct {
	grpc.ServerStream
	logger             log.Logger
	requestTranslator  matcher
	responseTranslator matcher
}

func (w *streamTranslator) RecvMsg(m any) error {
	if common.IsRequestTranslationDisabled(w.Context()) {
		return w.ServerStream.RecvMsg(m)
	}
	w.logger.Debug("Intercept RecvMsg", tag.NewAnyTag("message", m))
	changed, trErr := visitNamespace(m, w.requestTranslator)
	logTranslateNamespaceResult(w.logger, changed, trErr, "RecvMsg", m)
	return w.ServerStream.RecvMsg(m)
}

func (w *streamTranslator) SendMsg(m any) error {
	if common.IsRequestTranslationDisabled(w.Context()) {
		return w.ServerStream.SendMsg(m)
	}
	w.logger.Debug("Intercept SendMsg", tag.NewStringTag("type", fmt.Sprintf("%T", m)), tag.NewAnyTag("message", m))
	changed, trErr := visitNamespace(m, w.responseTranslator)
	logTranslateNamespaceResult(w.logger, changed, trErr, "SendMsg", m)
	return w.ServerStream.SendMsg(m)
}

func newStreamTranslator(
	s grpc.ServerStream,
	logger log.Logger,
	requestMapping map[string]string,
	responseMapping map[string]string,
) grpc.ServerStream {
	return &streamTranslator{
		ServerStream:       s,
		logger:             logger,
		requestTranslator:  createNameTranslator(requestMapping),
		responseTranslator: createNameTranslator(responseMapping),
	}
}

func logTranslateNamespaceResult(logger log.Logger, changed bool, err error, methodName string, obj any) {
	logger = log.With(
		logger,
		tag.NewStringTag("method", methodName),
		tag.NewAnyTag("obj", obj),
	)
	if err != nil {
		logger.Error("namespace translation error", tag.Error(err))
	} else if changed {
		logger.Debug("namespace translation applied")
	} else {
		logger.Debug("namespace translation not applied")
	}
}
