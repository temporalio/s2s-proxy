package proxy

import (
	"context"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/temporalio/s2s-proxy/collect"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/metrics"
)

func MakeServerOptions(
	logger log.Logger,
	directionLabel string,
	tlsConfig encryption.TLSConfig,
	aclPolicy *config.ACLPolicy,
	namespaceTranslation collect.StaticBiMap[string, string],
	searchAttributeTranslation config.SearchAttributeTranslation,
) ([]grpc.ServerOption, error) {
	var unaryInterceptors []grpc.UnaryServerInterceptor
	var streamInterceptors []grpc.StreamServerInterceptor
	labelGenerator := grpcprom.WithLabelsFromContext(func(_ context.Context) (labels prometheus.Labels) {
		return prometheus.Labels{"direction": directionLabel}
	})

	// Ordering matters! These metrics happen BEFORE the translations/acl
	unaryInterceptors = append(unaryInterceptors, metrics.GRPCServerMetrics.UnaryServerInterceptor(labelGenerator))
	streamInterceptors = append(streamInterceptors, metrics.GRPCServerMetrics.StreamServerInterceptor(labelGenerator))

	var translators []interceptor.Translator
	if namespaceTranslation.Len() > 0 {
		translators = append(translators, interceptor.NewNamespaceNameTranslator(logger, namespaceTranslation.AsMap(), namespaceTranslation.Inverse().AsMap()))
	}

	if searchAttributeTranslation.LenNamespaces() > 0 {
		logger.Info("search attribute translation enabled", tag.NewAnyTag("mappings", searchAttributeTranslation))
		if searchAttributeTranslation.LenNamespaces() > 1 {
			panic("multiple namespace search attribute mappings are not supported")
		}
		translators = append(translators, interceptor.NewSearchAttributeTranslator(logger, searchAttributeTranslation.FlattenMaps(), searchAttributeTranslation.Inverse().FlattenMaps()))
	}

	if len(translators) > 0 {
		tr := interceptor.NewTranslationInterceptor(logger, translators)
		unaryInterceptors = append(unaryInterceptors, tr.Intercept)
		streamInterceptors = append(streamInterceptors, tr.InterceptStream)
	}

	if aclPolicy != nil {
		aclInterceptor := interceptor.NewAccessControlInterceptor(logger, aclPolicy.AllowedMethods.AdminService, aclPolicy.AllowedNamespaces)
		unaryInterceptors = append(unaryInterceptors, aclInterceptor.Intercept)
		streamInterceptors = append(streamInterceptors, aclInterceptor.StreamIntercept)
	}

	opts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(unaryInterceptors...),
		grpc.ChainStreamInterceptor(streamInterceptors...),
	}

	if tlsConfig.IsEnabled() {
		tlsConfig, err := encryption.GetServerTLSConfig(tlsConfig, logger)
		if err != nil {
			return opts, err
		}
		opts = append(opts, grpc.Creds(credentials.NewTLS(tlsConfig)))
	}

	return opts, nil
}
