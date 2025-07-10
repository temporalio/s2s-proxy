
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
TEST_ARG ?= -race -timeout=5m
BENCH_ARG ?= -benchtime=5000x

ALL_SRC         := $(shell find . -name "*.go")
ALL_SRC         += go.mod

all: bins lint
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
	$(GO_GET_TOOL) github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1
	go mod tidy --modfile=$(TOOLS_MOD_FILE)

# Refer to .golangci.yml for configuration options
fmt:
	$(GO_TOOL) golangci-lint fmt

# Refer to .golangci.yml for configuration options
lint:
	@printf $(COLOR) "Running golangci-lint...\n"
	$(GO_TOOL) golangci-lint run

bench:
	@go test -run '^$$' -benchmem -bench=. ./... $(BENCH_ARG)

# Mocks
clean-mocks:
	@find . -name '*_mock.go' -delete

MOCKGEN_VER = v0.4.0
mocks: clean-mocks
	@if [ "$$(mockgen -version)" != "$(MOCKGEN_VER)" ]; then \
		echo -e "ERROR: mockgen is not version $(MOCKGEN_VER)\n"; \
		echo -e "  Run go install go.uber.org/mock/mockgen@$(MOCKGEN_VER)\n"; \
		echo -e "  Or, bump MOCKGEN_VER in the Makefile\n"; \
		exit 1; \
	fi;
	@mockgen -source config/config.go -destination mocks/config/config_mock.go -package config
	@mockgen -source client/temporal_client.go -destination mocks/client/temporal_client_mock.go -package client

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
	cp $(GENRPCWRAPPERS_DIR)/conversion_gen.go common/frontend_conversion_gen.go

	rm -rf ./cmd/tools/genrpcwrappers/*_gen.go
	cd $(GENRPCWRAPPERS_DIR); go run .  -service admin -license_file ../../../LICENSE
	cp ./cmd/tools/genrpcwrappers/lazy_client_gen.go client/admin/lazy_client_gen.go
	cp $(GENRPCWRAPPERS_DIR)/conversion_gen.go common/admin_conversion_gen.go

	rm -rf ./cmd/tools/genrpcwrappers/*_gen.go
	go fmt ./client/... ./common/...

test: generate-test-certs
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

build: clean-builds amd64-build

.PHONY: vendor-protos
vendor-protos:
	@if ! comby --version $> /dev/null ; then brew install comby; fi
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
