// Command bearer-token runs s2s-proxy with a CredentialProvider that attaches a bearer token to every call the proxy
// makes to its local Temporal server.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"

	"github.com/temporalio/s2s-proxy/app"
	"github.com/temporalio/s2s-proxy/auth"
)

const tokenEnvironmentVariable = "S2S_PROXY_EXAMPLE_BEARER_TOKEN"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application := app.New(
		"s2s-proxy-bearer-example",
		"dev",
		auth.WithCredentialProvider(bearerCredentialProvider{token: tokenFromEnvironment}),
	)
	if err := application.Run(ctx, os.Args); err != nil {
		panic(err)
	}
}

// bearerCredentialProvider is the auth.CredentialProvider the proxy is started with.
type bearerCredentialProvider struct {
	token func(context.Context) (string, error)
}

func (p bearerCredentialProvider) Get() credentials.PerRPCCredentials {
	return bearerCredentials(p)
}

// bearerCredentials sends "authorization: Bearer <token>". gRPC calls GetRequestMetadata for every unary call and
// every new stream, so a token that changes is picked up without recreating the connection.
type bearerCredentials struct {
	token func(context.Context) (string, error)
}

func (c bearerCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	token, err := c.token(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "bearer token is unavailable")
	}
	return map[string]string{"authorization": "Bearer " + token}, nil
}

// RequireTransportSecurity keeps the token off plaintext connections.
func (bearerCredentials) RequireTransportSecurity() bool {
	return true
}

func tokenFromEnvironment(context.Context) (string, error) {
	token := os.Getenv(tokenEnvironmentVariable)
	if token == "" {
		return "", status.Error(codes.Unauthenticated, tokenEnvironmentVariable+" is not set")
	}
	return token, nil
}
