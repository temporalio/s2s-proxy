
##### Arguments ######
GOOS          ?= $(shell go env GOOS)
GOARCH        ?= $(if $(TARGETARCH),$(TARGETARCH),$(shell go env GOARCH))
GOPATH        ?= $(shell go env GOPATH)
GOLANGCI_LINT ?= $(shell which golangci-lint)

TOOLS_MOD_FILE = develop/tools.mod
GO_TOOL        = go tool -modfile=$(TOOLS_MOD_FILE)
GO_GET_TOOL    = go get -tool -modfile=$(TOOLS_MOD_FILE)

# Disable cgo by default.
CGO_ENABLED ?= 0
TEST_ARG ?= -race -timeout=5m -tags test_dep -count=1
BENCH_ARG ?= -benchtime=5000x

ALL_SRC         := $(shell find . -name "*.go")
ALL_SRC         += go.mod

all: bins fmt lint
bins: s2s-proxy
clean:  clean-bins clean-tests

clean-bins:
	@printf $(COLOR) "Delete old binaries...\n"
	@rm -f ./bins/*

# Binary target
s2s-proxy: $(ALL_SRC)
	@printf $(COLOR) "Build s2s-proxy with CGO_ENABLED=$(CGO_ENABLED) for $(GOOS)/$(GOARCH)...\n"
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o ./bins/s2s-proxy ./cmd/proxy

update-tools:
# When changing the golangci-lint version, update the version in .github/workflows/pull-request.yml
	$(GO_GET_TOOL) github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8
	$(GO_GET_TOOL) go.uber.org/mock/mockgen@v0.5.0
	-go mod tidy --modfile=$(TOOLS_MOD_FILE) 2>/dev/null

# Refer to .golangci.yml for configuration options
fmt:
	$(GO_TOOL) golangci-lint fmt

# Refer to .golangci.yml for configuration options
lint:
	@printf $(COLOR) "Running golangci-lint...\n"
	$(GO_TOOL) golangci-lint run --build-tags=test_dep

bench:
	@go test -run '^$$' -benchmem -bench=. ./... $(BENCH_ARG)

.PHONY: genvisitor
GENVISITOR_FLAGS ?= # -debug -dump-tre
genvisitor:
	go run ./cmd/tools/genvisitor/ $(GENVISITOR_FLAGS) > proto/compat/repair_utf8_gen.go
	go fmt proto/compat/repair_utf8_gen.go
	make fmt

# Mocks
clean-mocks:
	@find . -name '*_mock.go' -delete

mocks: clean-mocks update-tools
	$(GO_TOOL) mockgen -source config/config.go -destination mocks/config/config_mock.go -package config
	$(GO_TOOL) mockgen -source client/temporal_client.go -destination mocks/client/temporal_client_mock.go -package client

# Tests
clean-tests:
	@printf $(COLOR) "Delete test certificates...\n"
	@rm -f develop/certificates/proxy1.* develop/certificates/proxy2.*

TEST_CERT=develop/certificates/proxy1.pem
$(TEST_CERT): scripts/generate-certs.sh
	@printf $(COLOR) "Generate test certificates...\n"
	./scripts/generate-certs.sh

generate-test-certs: $(TEST_CERT)

GENRPCWRAPPERS_DIR = ./cmd/tools/genrpcwrappers
generate-rpcwrappers:
	rm -rf $(GENRPCWRAPPERS_DIR)/*_gen.go
	cd $(GENRPCWRAPPERS_DIR); go run .  -service frontend -license_file ../../../LICENSE
	cp $(GENRPCWRAPPERS_DIR)/lazy_client_gen.go client/frontend/lazy_client_gen.go
	mkdir -p proto/compat
	cp $(GENRPCWRAPPERS_DIR)/conversion_gen.go proto/compat/frontend_conversion_gen.go

	rm -rf ./cmd/tools/genrpcwrappers/*_gen.go
	cd $(GENRPCWRAPPERS_DIR); go run .  -service admin -license_file ../../../LICENSE
	cp ./cmd/tools/genrpcwrappers/lazy_client_gen.go client/admin/lazy_client_gen.go
	cp $(GENRPCWRAPPERS_DIR)/conversion_gen.go proto/compat/admin_conversion_gen.go

	rm -rf ./cmd/tools/genrpcwrappers/*_gen.go
	go fmt ./client/... ./proto/compat/...

test: fmt lint generate-test-certs
	go test $(TEST_ARG) ./...

cover:
	go test $(TEST_ARG) -cover ./...

# arch build
%-build:
	mkdir -p build/linux/$*
	@GOOS=linux GOARCH=$* CGO_ENABLED=$(CGO_ENABLED) make clean-bins bins
	cp ./bins/* build/linux/$*

clean-builds:
	@printf $(COLOR) "Delete old builds...\n"
	@rm -rf ./build/*

build: clean-builds amd64-build arm64-build

.PHONY: vendor-protos
vendor-protos:
	@if ! comby --version &> /dev/null ; then brew install comby; fi
	./develop/vendor-protos.sh

# Docker
AWS_ECR_REGION ?=
AWS_ECR_PROFILE ?=
DOCKER_REPO ?=
DOCKER_TAG ?= $(shell whoami | tr -d " ")-local-$(shell git rev-parse --short HEAD)
DOCKER_IMAGE ?= temporal-s2s-proxy

.PHONY: docker-login
docker-login:
	@aws ecr get-login-password --region ${AWS_ECR_REGION} --profile "${AWS_ECR_PROFILE}" \
	| docker login --username AWS --password-stdin $(DOCKER_REPO)

.PHONY: docker-build-push
docker-build-push:
	@docker buildx build --platform=linux/amd64,linux/arm64 -t "${DOCKER_REPO}/${DOCKER_IMAGE}:${DOCKER_TAG}" --push .


.PHONY: helm-install
helm-install:
	brew install helm
	helm plugin install https://github.com/helm-unittest/helm-unittest.git

.PHONY: helm-test
helm-test:
	cd charts; helm unittest s2s-proxy/

.PHONY: helm-example
helm-example:
	cd charts; helm template example ./s2s-proxy -f ./s2s-proxy/values.example.yaml > s2s-proxy/example.yaml
	@echo "Example written to charts/s2s-proxy/example.yaml"
