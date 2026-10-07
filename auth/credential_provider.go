package auth

import (
	"go.uber.org/fx"
	"google.golang.org/grpc/credentials"
)

type (
	// CredentialProvider supplies the per-RPC credentials the proxy attaches to every call it makes to the local
	// Temporal server, for example a bearer token the server's authorizer expects.
	CredentialProvider interface {
		Get() credentials.PerRPCCredentials
	}

	// EmptyCredentialProvider is the default CredentialProvider. It supplies no credentials, so calls to the local
	// Temporal server carry none.
	EmptyCredentialProvider struct{}
)

// Module provides the CredentialProvider. It defaults to EmptyCredentialProvider; override it with
// WithCredentialProvider.
var Module = fx.Options(
	fx.Provide(func() CredentialProvider { return EmptyCredentialProvider{} }),
)

// WithCredentialProvider replaces the default CredentialProvider with the given one.
// Pass it to app.New as an extra fx option.
func WithCredentialProvider(provider CredentialProvider) fx.Option {
	return fx.Decorate(func(CredentialProvider) CredentialProvider { return provider })
}

func (EmptyCredentialProvider) Get() credentials.PerRPCCredentials {
	return nil
}

// IsEmptyCredentialProvider reports whether the provider supplies no credentials.
func IsEmptyCredentialProvider(provider CredentialProvider) bool {
	if provider == nil {
		return true
	}
	_, ok := provider.(EmptyCredentialProvider)
	return ok
}
