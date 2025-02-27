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

## Test locally

### Run proxy with inbound mode

Start Temporal server with sample configs

```
cd ${temporal_sever_folder}
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

## License

MIT License, please see [LICENSE](LICENSE) for details.