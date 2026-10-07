# Bearer-token credential provider example

This program runs s2s-proxy with a custom `auth.CredentialProvider`. The provider attaches
`authorization: Bearer <token>` to every call the proxy makes to its local Temporal server, so a server that runs a JWT
authorizer accepts the proxy. That covers AdminService, WorkflowService, OperatorService and replication streams. The
token is never sent to the remote side.

```shell
export S2S_PROXY_EXAMPLE_BEARER_TOKEN='replace-with-a-test-token'
go run ./examples/bearer-token start --config ./examples/bearer-token/config.yaml
```

## How it plugs in

The stock proxy uses `auth.EmptyCredentialProvider`, which sends no credentials. This example replaces it by passing
`auth.WithCredentialProvider` to `app.New`:

```go
app.New("s2s-proxy-bearer-example", "dev",
    auth.WithCredentialProvider(bearerCredentialProvider{token: tokenFromEnvironment}),
)
```

`Get` returns a gRPC `credentials.PerRPCCredentials`. gRPC calls its `GetRequestMetadata` for every unary call and every
new stream, so a rotated token takes effect on the next call without restarting the proxy or recreating connections.

## Configuration requirements

The local connection (`local.tcpClient`) must be `tcp` with TLS configured. `RequireTransportSecurity` returns true, and
the proxy refuses to start rather than send the token in plaintext. The sample `config.yaml` uses reserved `.invalid`
hostnames and placeholder certificate paths.

## Beyond the example

Reading the token from an environment variable is for demonstration only. A production provider gets short-lived
tokens from the operator's identity system, for example an OAuth2 client-credentials grant against Microsoft Entra ID,
and handles caching, refresh before expiry, retries, and a timeout on each fetch.

The identity the token represents must be allowed to call the APIs the proxy uses. Temporal treats every AdminService
method as a cluster-scoped admin operation, so replication requires admin-level access on the local server.
