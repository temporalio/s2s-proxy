.PHONY: clean bins

##### Arguments ######
GOOS          ?= $(shell go env GOOS)
GOARCH        ?= $(shell go env GOARCH)
GOPATH        ?= $(shell go env GOPATH)
GOLANGCI_LINT ?= $(shell which golangci-lint)

# Disable cgo by default.
CGO_ENABLED ?= 0

ALL_SRC         := $(shell find . -name "*.go")
ALL_SRC         += go.mod

all: bins lint
bins: s2s-proxy
clean:  clean-bins

clean-bins:
	@printf $(COLOR) "Delete old binaries..."
	@rm -f s2s-proxy

# Binary target
s2s-proxy: $(ALL_SRC)
	@printf $(COLOR) "Build s2s-proxy with CGO_ENABLED=$(CGO_ENABLED) for $(GOOS)/$(GOARCH)..."
	CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_TAG_FLAG) -o s2s-proxy ./cmd/proxy

# Lint target
lint:
	@printf $(COLOR) "Running golangci-lint..."
	@$(GOLANGCI_LINT) run 

clean-mocks:
	@find . -name '*_mock.go' -delete

mocks: clean-mocks
	@mockgen -source config/config.go -destination mocks/config/config_mock.go -package config