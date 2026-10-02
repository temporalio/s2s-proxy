# Bearer-token provider example

This executable registers an `example-bearer` outbound call-credentials provider and attaches its credential only to the configured destination.

```shell
export S2S_PROXY_EXAMPLE_BEARER_TOKEN='replace-with-a-test-token'
go run ./examples/bearer-token start --config ./examples/bearer-token/config.yaml
```

The token is read for each new RPC or stream and is never part of the YAML configuration. The sample configuration uses reserved `.invalid` hostnames and placeholder certificate paths.

This provider is for integration examples only. A production provider should acquire short-lived credentials from the operator's identity system and own caching, expiration-aware refresh, retries, and rotation.
