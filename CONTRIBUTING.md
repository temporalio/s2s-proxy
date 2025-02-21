# Develop

## Prerequisites

### Build prerequisites

- [Go Lang](https://go.dev/) (minimum version required listed in `go.mod` [file](go.mod)):
  - Install on macOS with `brew install go`.
  - Install on Ubuntu with `sudo apt install golang`.

### Local test prerequisites

- Download and run [Temporal server](https://github.com/temporalio/temporal/tree/main) locally. 

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

## License

MIT License, please see [LICENSE](LICENSE) for details.