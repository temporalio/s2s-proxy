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

## License

MIT License, please see [LICENSE](LICENSE) for details.