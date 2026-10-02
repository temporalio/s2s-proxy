package outboundauth

import (
	"context"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

type TokenProvider interface {
	GetToken(context.Context) (string, error)
}

type TokenProviderFunc func(context.Context) (string, error)

func (f TokenProviderFunc) GetToken(ctx context.Context) (string, error) {
	return f(ctx)
}

func NewBearerCredentials(provider TokenProvider) credentials.PerRPCCredentials {
	return &bearerCredentials{provider: provider}
}

type bearerCredentials struct {
	provider TokenProvider
}

func (c *bearerCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	if c.provider == nil {
		return nil, status.Error(codes.Internal, "outbound authentication credential provider is invalid")
	}
	token, err := c.provider.GetToken(ctx)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, status.Error(codes.Unauthenticated, "outbound authentication credential unavailable")
	}
	if strings.HasPrefix(strings.ToLower(token), "bearer ") || containsControlCharacters(token) {
		return nil, status.Error(codes.Internal, "outbound authentication credential is invalid")
	}
	return map[string]string{"authorization": "Bearer " + token}, nil
}

func (*bearerCredentials) RequireTransportSecurity() bool {
	return true
}

func containsControlCharacters(value string) bool {
	for _, char := range []byte(value) {
		if char < 0x20 || char == 0x7f {
			return true
		}
	}
	return false
}
