.PHONY: clean bins

##### Arguments ######
GOOS          ?= $(shell go env GOOS)
GOARCH        ?= $(shell go env GOARCH)
GOPATH        ?= $(shell go env GOPATH)
GOLANGCI_LINT ?= $(shell which golangci-lint)

# Disable cgo by default.
CGO_ENABLED ?= 0
TEST_ARG ?= -race -timeout=5m 

ALL_SRC         := $(shell find . -name "*.go")
ALL_SRC         += go.mod

all: bins lint
bins: s2s-proxy
clean:  clean-bins

clean-bins:
	@printf $(COLOR) "Delete old binaries...\n"
	@rm -f s2s-proxy

# Binary target
s2s-proxy: $(ALL_SRC)
	@printf $(COLOR) "Build s2s-proxy with CGO_ENABLED=$(CGO_ENABLED) for $(GOOS)/$(GOARCH)...\n"
	CGO_ENABLED=$(CGO_ENABLED) go build -o s2s-proxy ./cmd/proxy

# Lint target
lint:
	@printf $(COLOR) "Running golangci-lint...\n"
	@$(GOLANGCI_LINT) run 

# Mocks
clean-mocks:
	@find . -name '*_mock.go' -delete

mocks: clean-mocks
	@mockgen -source config/config.go -destination mocks/config/config_mock.go -package config

# Tests
test:
	go test $(TEST_ARG) ./...

cover:
	go test $(TEST_ARG) -cover ./...
