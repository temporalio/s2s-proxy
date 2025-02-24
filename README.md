# s2s-proxy

A server-to-server proxy (s2s-proxy) is a service sitting between two Temporal servers. It can be used to intercept service requests between two servers in a [multi-cluster setting](https://docs.temporal.io/self-hosted-guide/multi-cluster-replication). It enables communication between two Temporal servers even if the servers are located in segregated networks. The proxy can be customized with access control policies, such as defining allow lists for APIs and namespaces, to enhance security. Using multiplex mode allows one server to connect to another server unidirectionally unlike a typical multi-cluster setup where both servers must expose an accessible endpoint.

## License

[MIT License](https://github.com/temporalio/s2s-proxy/blob/main/LICENSE)