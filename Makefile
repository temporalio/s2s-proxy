.PHONY: clean bins

##### Arguments ######
GOOS        ?= $(shell go env GOOS)
GOARCH      ?= $(shell go env GOARCH)
GOPATH      ?= $(shell go env GOPATH)
# Disable cgo by default.
CGO_ENABLED ?= 0

ALL_SRC         := $(shell find . -name "*.go")
ALL_SRC         += go.mod

bins: temporal-proxy
clean:  clean-bins

clean-bins:
	@printf $(COLOR) "Delete old binaries..."
	@rm -f temporal-proxy

temporal-proxy: $(ALL_SRC)
	@printf $(COLOR) "Build temporal-proxy with CGO_ENABLED=$(CGO_ENABLED) for $(GOOS)/$(GOARCH)..."
	CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_TAG_FLAG) -o temporal-proxy ./cmd/proxy

