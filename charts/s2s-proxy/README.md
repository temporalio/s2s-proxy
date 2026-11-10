Overview
--------

This Helm chart deploys the s2s-proxy.

It creates the following Kubernetes resources:

* ConfigMap containing the s2s-proxy config
* Deployment to deploy s2s-proxy pod(s)
* Service to load balancer across the s2s-proxy outbound server(s)


Configuration
-------------

Use the `configOverride` field in values.yaml to override arbitrary values in the s2s-proxy config file. These overrides are merged on top of the
[files/defaults.yaml](/charts/s2s-proxy/files/default.yaml).

Below is an example `configOverride` setup for configuring s2s-proxy during a namespace migration to Temporal Cloud:

```yaml
configOverride:
  clusterConnections:
    - name: "my-migration-cluster"
      local:
        tcpClient:
          # Address of your Temporal server's frontend.
          address: "frontend-address:7233"
      remote:
        muxAddressInfo:
          # Address of the migration endpoint.
          address: "s2s-proxy-sample.example.tmprl.cloud:8233"
          tls:
            # Path to your client certificate for mTLS authentication.
            certificatePath: "/s2c-server-tls/tls.crt"
            # Path to your private key corresponding to the client certificate.
            keyPath: "/s2c-server-tls/tls.key"
      # Your s2s-proxy service address. This will be called by your Temporal server during namespace migration.
      replicationEndpoint: "address-of-your-s2s-proxy-deployment:9233"
      namespaceTranslation:
        mappings:
          - local: my-local       # Name of the namespace in your self-hosted Temporal.
            remote: my-cloud.acct # Corresponding namespace pre-created in Temporal Cloud.
```

### gRPC keepalive

The default config turns on HTTP/2 keepalive so connections that die silently are closed within about a minute,
instead of the ~15 minutes TCP takes:

* `local.keepalive.client`: the proxy pings your Temporal frontend every 30s. Temporal frontends accept a ping every 10s by default.
* `local.keepalive.server`: the proxy accepts a ping every 10s from your Temporal server, and pings it after 1m without traffic.
* `remote.keepalive.server`: the same, for the gRPC traffic the remote proxy sends inside the mux.

Pings from this proxy to the remote proxy (`remote.keepalive.client`) stay off, because the remote proxy closes the
connection with GOAWAY `too_many_pings` unless it accepts them. Turn them on once the remote proxy runs with
`remote.keepalive.server.enforcementPolicy`.

To let your Temporal server ping the proxy, set this in its static config and turn on the
`system.enableInternodeClientKeepAlive` dynamic config:

```yaml
services:
  frontend:
    rpc:
      clientConnectionConfig:
        keepAliveClientParameters:
          keepAliveTime: "30s"
          keepAliveTimeout: "20s"
          keepAlivePermitWithoutStream: true
```

To change a value, set it in `configOverride`. `configOverride` is merged over the defaults, so a value set to `null`,
`0` or `false` there falls back to the default rather than removing it. To stop the proxy pinging your frontend, set
`local.keepalive.client.time: 0s`.

Generate example helm chart
---------------------------

```
make helm-example
```

Testing
-------

## Unit tests

Install [helm-unittest](https://github.com/helm-unittest/helm-unittest)

```
make helm-test
```
