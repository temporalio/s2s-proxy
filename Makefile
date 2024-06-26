.PHONY: clean bins

##### Arguments ######
GOOS        ?= $(shell go env GOOS)
GOARCH      ?= $(shell go env GOARCH)
GOPATH      ?= $(shell go env GOPATH)
# Disable cgo by default.
CGO_ENABLED ?= 0

ALL_SRC         := $(shell find . -name "*.go")
ALL_SRC         += go.mod

bins: s2s-proxy
clean:  clean-bins

clean-bins:
	@printf $(COLOR) "Delete old binaries..."
	@rm -f s2s-proxy

s2s-proxy: $(ALL_SRC)
	@printf $(COLOR) "Build s2s-proxy with CGO_ENABLED=$(CGO_ENABLED) for $(GOOS)/$(GOARCH)..."
	CGO_ENABLED=$(CGO_ENABLED) go build $(BUILD_TAG_FLAG) -o s2s-proxy ./cmd/proxy

# TODO: linting / static checks