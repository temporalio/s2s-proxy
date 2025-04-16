package interceptor

import (
	"context"
	"fmt"
	"strings"

	"github.com/temporalio/s2s-proxy/config"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
)

type (
	ClusterNameTranslator struct {
		logger              log.Logger
		requestNameMapping  map[string]string
		responseNameMapping map[string]string
	}
)

func NewClusterNameTranslator(
	logger log.Logger,
	cfg config.ProxyConfig,
	isInbound bool,
	nameTranslations config.NameTranslationConfig,
) *ClusterNameTranslator {
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

	return &ClusterNameTranslator{
		logger:              logger,
		requestNameMapping:  requestNameMapping,
		responseNameMapping: responseNameMapping,
	}
}

var _ grpc.UnaryServerInterceptor = (*ClusterNameTranslator)(nil).Intercept
var _ grpc.StreamServerInterceptor = (*ClusterNameTranslator)(nil).InterceptStream

func (i *ClusterNameTranslator) Intercept(
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
		changed, trErr := visitClusterName(req, createNameTranslator(i.requestNameMapping))
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Request", req)

		resp, err := handler(ctx, req)

		// Translate namespace name in response.
		changed, trErr = visitClusterName(resp, createNameTranslator(i.responseNameMapping))
		logTranslateNamespaceResult(i.logger, changed, trErr, methodName+"Response", resp)
		return resp, err
	} else {
		return handler(ctx, req)
	}
}

func (i *ClusterNameTranslator) InterceptStream(
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
	err := handler(srv, newClusterStreamTranslator(
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

type clusterStreamTranslator struct {
	grpc.ServerStream
	logger             log.Logger
	requestTranslator  matcher
	responseTranslator matcher
}

func (w *clusterStreamTranslator) RecvMsg(m any) error {
	w.logger.Debug("Intercept RecvMsg", tag.NewAnyTag("message", m))
	changed, trErr := visitClusterName(m, w.requestTranslator)
	logTranslateClusterResult(w.logger, changed, trErr, "RecvMsg", m)
	return w.ServerStream.RecvMsg(m)
}

func (w *clusterStreamTranslator) SendMsg(m any) error {
	w.logger.Debug("Intercept SendMsg", tag.NewStringTag("type", fmt.Sprintf("%T", m)), tag.NewAnyTag("message", m))
	changed, trErr := visitClusterName(m, w.responseTranslator)
	logTranslateClusterResult(w.logger, changed, trErr, "SendMsg", m)
	return w.ServerStream.SendMsg(m)
}

func newClusterStreamTranslator(
	s grpc.ServerStream,
	logger log.Logger,
	requestMapping map[string]string,
	responseMapping map[string]string,
) grpc.ServerStream {
	return &clusterStreamTranslator{
		ServerStream:       s,
		logger:             logger,
		requestTranslator:  createNameTranslator(requestMapping),
		responseTranslator: createNameTranslator(responseMapping),
	}
}

func logTranslateClusterResult(logger log.Logger, changed bool, err error, methodName string, obj any) {
	logger = log.With(
		logger,
		tag.NewStringTag("method", methodName),
		tag.NewAnyTag("obj", obj),
	)
	if err != nil {
		logger.Error("cluster name translation error", tag.Error(err))
	} else if changed {
		logger.Debug("cluster name translation applied")
	} else {
		logger.Debug("cluster name translation not applied")
	}
}
