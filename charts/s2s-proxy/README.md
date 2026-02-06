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
