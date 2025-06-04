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

For example, to configure the mux `serverAddress`:

```yaml
configOverride:
  mux:
    client:
       serverAddress: "s2s-proxy-endpoint.example.tmprl.cloud:8233"
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
