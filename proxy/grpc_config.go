package proxy

import (
	"context"

	grpcprom "github.com/grpc-ecosystem/go-grpc-middleware/providers/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/temporalio/s2s-proxy/config"
	"github.com/temporalio/s2s-proxy/encryption"
	"github.com/temporalio/s2s-proxy/interceptor"
	"github.com/temporalio/s2s-proxy/metrics"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const Inbound string = "inbound"
const Outbound string = "outbound"

func makeServerOptions(
	logger log.Logger,
	isInbound bool,
	tlsConfig encryption.ServerTLSConfig,
	aclPolicy *config.ACLPolicy,
	namespaceTranslation config.NameTranslationConfig,
	searchAttributeTranslation config.SATranslationConfig,
) ([]grpc.ServerOption, error) {
	var unaryInterceptors []grpc.UnaryServerInterceptor
	var streamInterceptors []grpc.StreamServerInterceptor

	directionLabel := "outbound"
	if isInbound {
		directionLabel = "inbound"
	}
	labelGenerator := grpcprom.WithLabelsFromContext(func(_ context.Context) (labels prometheus.Labels) {
		return prometheus.Labels{"direction": directionLabel}
	})

	// Ordering matters! These metrics happen BEFORE the translations/acl
	unaryInterceptors = append(unaryInterceptors, metrics.GRPCServerMetrics.UnaryServerInterceptor(labelGenerator))
	streamInterceptors = append(streamInterceptors, metrics.GRPCServerMetrics.StreamServerInterceptor(labelGenerator))

	var translators []interceptor.Translator
	if namespaceTranslation.IsEnabled() {
		// NamespaceNameTranslator needs to be called before namespace access control so that
		// local name can be used in namespace allowed list.
		reqMap, respMap := namespaceTranslation.ToMaps(isInbound)
		translators = append(translators, interceptor.NewNamespaceNameTranslator(logger, reqMap, respMap))
	}

	if searchAttributeTranslation.IsEnabled() {
		logger.Info("search attribute translation enabled", tag.NewAnyTag("mappings", searchAttributeTranslation.NamespaceMappings))
		if len(searchAttributeTranslation.NamespaceMappings) > 1 {
			panic("multiple namespace search attribute mappings are not supported")
		}
		reqMap, respMap := searchAttributeTranslation.ToMaps(isInbound)
		translators = append(translators, interceptor.NewSearchAttributeTranslator(logger, reqMap, respMap))
	}

	if len(translators) > 0 {
		tr := interceptor.NewTranslationInterceptor(logger, translators)
		unaryInterceptors = append(unaryInterceptors, tr.Intercept)
		streamInterceptors = append(streamInterceptors, tr.InterceptStream)
	}

	if isInbound && aclPolicy != nil {
		aclInterceptor := interceptor.NewAccessControlInterceptor(logger, aclPolicy)
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
