# Develop

## Prerequisites

### Supported platforms

OS: Linux, macOS
Arch: amd64, arm64

### Build prerequisites

- [Go Lang](https://go.dev/) (minimum version required listed in `go.mod` [file](go.mod)):
  - Install on macOS with `brew install go`.
  - Install on Ubuntu with `sudo apt install golang`.

### Local test prerequisites

- Download and run [Temporal server](https://github.com/temporalio/temporal/tree/main) locally.
- Install [Temporal CLI](https://github.com/temporalio/cli).

## Build

Build all

```bash
make
```

Build binaries without running tests with:

```bash
make bins
```

Run tests

```
make test
```

Build and push docker image

```
make docker-login AWS_ECR_REGION=${region} AWS_ECR_PROFILE=${profile} DOCKER_REPO=${ecr_repro}

make docker-build-push AWS_ECR_REGION=${region} AWS_ECR_PROFILE=${profile} DOCKER_REPO=${ecr_repro}
```

## Test locally

### Run proxy with inbound mode

Start Temporal server with sample configs

```
cd ${temporal_server_folder}
make start-dependencies
make bins
./temporal-server --config ${s2s_proxy_folder}/develop/config --env cluster-a --allow-no-auth start
```

Start s2s-proxy

```
./bins/s2s-proxy start --config develop/config/cluster-a-tcp-inbound-proxy.yaml
```

Run
```
temporal operator --address 127.0.0.1:6233 cluster describe
```

Request will be forwarded from proxy (port :6233) to local server (port :7233).

### Connect two Temporal servers via s2s-proxies with multiplexing ("mux") mode
In multiplexing mode, one proxy establishes a TCP connection to the other proxy, and Temporal server connections are multiplexed over that single connection.
Start servers
```
./temporal-server --config ${s2s_proxy_folder}/develop/config --env cluster-a --allow-no-auth start
./temporal-server --config ${s2s_proxy_folder}/develop/config --env cluster-b --allow-no-auth start
```

Start proxies
```
./bins/s2s-proxy start --config develop/config/cluster-a-mux-client-proxy.yaml
./bins/s2s-proxy start --config develop/config/cluster-b-mux-server-proxy.yaml
```

## Code Generation

### gRPC client generation

Run `make generate-rpcwrappers` to re-generate the clients

This uses the `cmd/tools/genrpcwrappers` tool to generates frontend and admin clients.

### Invalid UTF-8 Repair Functions

The proxy supports automatically repairing invalid UTF-8 strings in Temporal protobuf messages.

Background: Invalid UTF-8 strings could appear in protobuf messages in older versions of Temporal (<=1.22) which used [gogo/protobuf](https://github.com/gogo/protobuf). Newer versions of Temporal use `google.golang.org/protobuf` which validates UTF-8 strings during serialization. This means any messages with invalid UTF-8 strings cannot be processed by s2s-proxy or by newer Temporal server versions. To fix this, s2s-proxy automatically repairs invalid UTF-8 strings by rewriting messages as the pass through the proxy. It does this by including a copy of the old gogo-based protos and when an invalid UTF-8 error is seen during protobuf deserialization, it can use the gogo-based protos to deserialize the message and repair the invalid UTF-8 string.

Code generation is used to generate functions to handle all possible cases:

* `make generate-rpcwrappers` to re-generates type conversion functions
* `make genvisitor` re-generates the invalid UTF-8 repair functions

Both of these code generation tools are based on protobuf reflection.

## License

MIT License, please see [LICENSE](LICENSE) for details.