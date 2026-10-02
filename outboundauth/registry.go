package outboundauth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/metrics"
)

var (
	providerNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	metadataKeyPattern  = regexp.MustCompile(`^[0-9a-z_.-]+$`)
)

var reservedMetadataKeys = map[string]struct{}{
	"content-type":               {},
	"te":                         {},
	"user-agent":                 {},
	"s2s-request-translation":    {},
	"xdc-redirection":            {},
	"x-s2s-intra-proxy":          {},
	"x-s2s-origin-proxy-id":      {},
	"x-s2s-hop-count":            {},
	"x-s2s-trace-id":             {},
	"temporal-client-cluster-id": {},
	"temporal-client-shard-id":   {},
	"temporal-server-cluster-id": {},
	"temporal-server-shard-id":   {},
	"temporal-principal-type":    {},
	"temporal-principal-name":    {},
}

type Registry struct {
	factories map[string]Factory
}

func NewRegistry(registrations ...Registration) (*Registry, error) {
	registry := &Registry{factories: make(map[string]Factory, len(registrations))}
	for _, registration := range registrations {
		if !providerNamePattern.MatchString(registration.Name) {
			return nil, fmt.Errorf("invalid outbound call credentials provider name %q", registration.Name)
		}
		if registration.Factory == nil {
			return nil, fmt.Errorf("outbound call credentials provider %q has no factory", registration.Name)
		}
		if factory, ok := registration.Factory.(FactoryFunc); ok && factory == nil {
			return nil, fmt.Errorf("outbound call credentials provider %q has no factory", registration.Name)
		}
		if _, exists := registry.factories[registration.Name]; exists {
			return nil, fmt.Errorf("duplicate outbound call credentials provider %q", registration.Name)
		}
		registry.factories[registration.Name] = registration.Factory
	}
	return registry, nil
}

func (r *Registry) Build(name string, request BuildRequest, logger log.Logger) (BuiltCredentials, error) {
	factory, ok := r.factories[name]
	if !ok {
		return BuiltCredentials{}, fmt.Errorf("unknown outbound call credentials provider %q", name)
	}

	request.Properties = cloneMap(request.Properties)
	built, err := factory.Build(request)
	if err != nil {
		return BuiltCredentials{}, fmt.Errorf("build outbound call credentials provider %q: %w", name, err)
	}
	if built.PerRPC == nil {
		return BuiltCredentials{}, fmt.Errorf("outbound call credentials provider %q returned nil credentials", name)
	}

	ownedKeys, err := normalizeOwnedKeys(built.OwnedMetadataKeys)
	if err != nil {
		return BuiltCredentials{}, fmt.Errorf("outbound call credentials provider %q: %w", name, err)
	}

	return BuiltCredentials{
		PerRPC: &validatedCredentials{
			delegate:    built.PerRPC,
			ownedKeys:   toSet(ownedKeys),
			provider:    name,
			destination: request.Target.Destination,
			logger:      logger,
		},
		OwnedMetadataKeys: ownedKeys,
	}, nil
}

func cloneMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func normalizeOwnedKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, errors.New("no owned metadata keys declared")
	}

	result := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		normalized := strings.ToLower(key)
		if err := validateMetadataKey(normalized); err != nil {
			return nil, err
		}
		if _, duplicate := seen[normalized]; duplicate {
			return nil, fmt.Errorf("duplicate owned metadata key %q", normalized)
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func validateMetadataKey(key string) error {
	if !metadataKeyPattern.MatchString(key) {
		return fmt.Errorf("invalid metadata key %q", key)
	}
	if strings.HasPrefix(key, "grpc-") {
		return fmt.Errorf("reserved metadata key %q", key)
	}
	if _, reserved := reservedMetadataKeys[key]; reserved {
		return fmt.Errorf("reserved metadata key %q", key)
	}
	return nil
}

func toSet(keys []string) map[string]struct{} {
	result := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		result[key] = struct{}{}
	}
	return result
}

type validatedCredentials struct {
	delegate    credentials.PerRPCCredentials
	ownedKeys   map[string]struct{}
	provider    string
	destination Destination
	logger      log.Logger
}

func (c *validatedCredentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	start := time.Now()
	metadataValues, err := c.delegate.GetRequestMetadata(ctx, uri...)
	if err != nil {
		metrics.CallCredentialsRequests.WithLabelValues(c.provider, string(c.destination), "error").Inc()
		metrics.CallCredentialsLatency.WithLabelValues(c.provider, string(c.destination)).Observe(time.Since(start).Seconds())
		code := providerErrorCode(ctx, err)
		c.logger.Warn("outbound call credential resolution failed",
			tag.NewStringTag("provider", c.provider),
			tag.NewStringTag("destination", string(c.destination)),
			tag.NewStringTag("code", code.String()))
		return nil, status.Error(code, "outbound authentication credential unavailable")
	}

	validated, validationErr := c.validateResult(metadataValues)
	outcome := "success"
	if validationErr != nil {
		outcome = "error"
		if status.Code(validationErr) == codes.Unauthenticated {
			outcome = "empty"
		}
		c.logger.Warn("outbound call credential validation failed",
			tag.NewStringTag("provider", c.provider),
			tag.NewStringTag("destination", string(c.destination)),
			tag.NewStringTag("code", status.Code(validationErr).String()))
	}
	metrics.CallCredentialsRequests.WithLabelValues(c.provider, string(c.destination), outcome).Inc()
	metrics.CallCredentialsLatency.WithLabelValues(c.provider, string(c.destination)).Observe(time.Since(start).Seconds())
	return validated, validationErr
}

func (c *validatedCredentials) validateResult(values map[string]string) (map[string]string, error) {
	if len(values) == 0 {
		return nil, status.Error(codes.Unauthenticated, "outbound authentication credential unavailable")
	}

	result := make(map[string]string, len(values))
	for key, value := range values {
		normalized := strings.ToLower(key)
		if _, owned := c.ownedKeys[normalized]; !owned {
			return nil, status.Error(codes.Internal, "outbound authentication credential is invalid")
		}
		if _, duplicate := result[normalized]; duplicate {
			return nil, status.Error(codes.Internal, "outbound authentication credential is invalid")
		}
		if value == "" {
			return nil, status.Error(codes.Unauthenticated, "outbound authentication credential unavailable")
		}
		if !validMetadataValue(normalized, value) {
			return nil, status.Error(codes.Internal, "outbound authentication credential is invalid")
		}
		result[normalized] = value
	}
	return result, nil
}

func (*validatedCredentials) RequireTransportSecurity() bool {
	return true
}

func validMetadataValue(key string, value string) bool {
	if strings.HasSuffix(key, "-bin") {
		return true
	}
	for _, char := range []byte(value) {
		if char < 0x20 || char > 0x7e {
			return false
		}
	}
	return true
}

func providerErrorCode(ctx context.Context, err error) codes.Code {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return status.FromContextError(ctxErr).Code()
	}
	code := status.Code(err)
	if code == codes.Canceled || code == codes.DeadlineExceeded || code == codes.Unavailable {
		return code
	}
	return codes.Unavailable
}
