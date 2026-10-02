package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/temporalio/s2s-proxy/app"
	"github.com/temporalio/s2s-proxy/outboundauth"
)

const tokenEnvironmentVariable = "S2S_PROXY_EXAMPLE_BEARER_TOKEN"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application := app.New(
		"s2s-proxy-bearer-example",
		"dev",
		app.WithCallCredentialsProvider("example-bearer", bearerFactory()),
	)
	if err := application.Run(ctx, os.Args); err != nil {
		panic(err)
	}
}

func bearerFactory() outboundauth.Factory {
	return outboundauth.FactoryFunc(func(outboundauth.BuildRequest) (outboundauth.BuiltCredentials, error) {
		credentials := outboundauth.NewBearerCredentials(outboundauth.TokenProviderFunc(tokenFromEnvironment))
		return outboundauth.BuiltCredentials{
			PerRPC:            credentials,
			OwnedMetadataKeys: []string{"authorization"},
		}, nil
	})
}

func tokenFromEnvironment(context.Context) (string, error) {
	token, ok := os.LookupEnv(tokenEnvironmentVariable)
	if !ok || token == "" {
		return "", errors.New("example bearer token is unavailable")
	}
	return token, nil
}
