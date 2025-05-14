# s2s-proxy

A server-to-server proxy (s2s-proxy) is a service sitting between two Temporal servers. It can be used to intercept service requests between two servers in a [multi-cluster setting](https://docs.temporal.io/self-hosted-guide/multi-cluster-replication). It enables communication between two Temporal servers even if the servers are located in segregated networks. The proxy can be customized with access control policies, such as defining allow lists for APIs and namespaces, to enhance security. Using multiplex mode allows one server to connect to another server unidirectionally unlike a typical multi-cluster setup where both servers must expose an accessible endpoint.

*This project is intended for use as a binary only.*

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for how to build and run locally, run tests, etc.

## License

[MIT License](https://github.com/temporalio/s2s-proxy/blob/main/LICENSE)


## Usage Instructions

1. Clone the Temporal s2s-proxy repository:
   ```
   git clone https://github.com/temporalio/s2s-proxy.git
   cd s2s-proxy
   ```

2. Build the Docker image:
   ```
   docker build -t your-custom-s2s-proxy:latest .
   ```

3. Push the Docker image to your container registry:
   ```
   docker tag your-custom-s2s-proxy:latest your-registry/your-custom-s2s-proxy:latest
   docker push your-registry/your-custom-s2s-proxy:latest
   ```

4. Update the `values.yaml` file to point to your image repository:
   ```yaml
   image:
     repository: "your-registry/your-custom-s2s-proxy"
     tag: "latest"
   ```

5. Deploy the Helm chart:
   ```
   helm install s2s-proxy ./s2s-proxy -f values.yaml
   ```

## Configuration

The s2s-proxy Helm chart can be configured using the following values:

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of replicas | `1` |
| `image.repository` | Image repository | `"your-custom-s2s-proxy"` |
| `image.pullPolicy` | Image pull policy | `"IfNotPresent"` |
| `image.tag` | Image tag | `"latest"` |
| `config.source.hostPort` | Source Temporal server host:port | `"temporal-frontend:7233"` |
| `config.target.hostPort` | Target Temporal server host:port | `"target-temporal-frontend:7233"` |
| `config.multiplexMode` | Enable multiplex mode | `false` |
| `config.logLevel` | Log level (debug, info, warn, error) | `"info"` |
| `config.accessControl.enabled` | Enable access control | `false` |
| `config.accessControl.apiAllowList` | List of allowed APIs | `[]` |
| `config.accessControl.namespaceAllowList` | List of allowed namespaces | `[]` |
| `service.type` | Service type | `"ClusterIP"` |
| `service.port` | Service port | `7233` |

## Notes

- The s2s-proxy enables communication between two Temporal servers even if they are located in segregated networks
- It can be customized with access control policies to enhance security
- Multiplex mode allows one server to connect to another unidirectionally (unlike a typical multi-cluster setup)





