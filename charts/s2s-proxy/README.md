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
  inbound:
    client:
      tcp:
        # Address of your Temporal server's frontend.
        serverAddress: "frontend-address:7233" 
  outbound:
    server:
      tcp:
        # Your s2s-proxy service address, used as the address of migration server.
        externalAddress: "address-of-your-s2s-proxy-deployment:9233" 
  mux:
    - client:
        # Address of the migration endpoint.
        serverAddress: "s2s-proxy-sample.example.tmprl.cloud:8233"
        tls:
          # Path to your client certificate for mTLS authentication.
          certificatePath: "/s2c-server-tls/tls.crt"
          # Path to your private key corresponding to the client certificate.
          keyPath: "/s2c-server-tls/tls.key"
  namespaceNameTranslation:
    mappings:
    - localName: my-local       # Name of the namespace in your self-hosted Temporal.
      remoteName: my-cloud.acct # Corresponding namespace pre-created in Temporal Cloud.
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
