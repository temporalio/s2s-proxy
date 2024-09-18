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
clean:  clean-bins clean-tests

clean-bins:
	@printf $(COLOR) "Delete old binaries...\n"
	@rm -f ./bins/*

# Binary target
s2s-proxy: $(ALL_SRC)
	@printf $(COLOR) "Build s2s-proxy with CGO_ENABLED=$(CGO_ENABLED) for $(GOOS)/$(GOARCH)...\n"
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=$(CGO_ENABLED) go build -o ./bins/s2s-proxy ./cmd/proxy

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
clean-tests:
	@printf $(COLOR) "Delete test certificates...\n"
	@rm -f develop/certificates/proxy1.* develop/certificates/proxy2.*

TEST_CERT=develop/certificates/proxy1.pem
$(TEST_CERT): scripts/generate-certs.sh
	@printf $(COLOR) "Generate test certificates...\n"
	./scripts/generate-certs.sh

generate-test-certs: $(TEST_CERT)

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

# Docker
DOCKER_REPO ?= 612212029444.dkr.ecr.us-west-2.amazonaws.com
DOCKER_TAG ?= $(shell whoami | tr -d " ")-local-$(shell git rev-parse --short HEAD)
DOCKER_IMAGE ?= temporal-s2s-proxy

# amd64 build only
.PHONY: docker-build
docker-build: build
	@docker build --platform=linux/amd64 . --tag "${DOCKER_REPO}/${DOCKER_IMAGE}:${DOCKER_TAG}"

.PHONY: docker-login
docker-login:
	@omni aws ecr get-login-password --region us-west-2 --profile temporal/AWSAdministratorAccess \
	| docker login --username AWS --password-stdin $(DOCKER_REPO)

.PHONY: docker-push
docker-push: ## Push to repository
	@docker push ${DOCKER_REPO}/${DOCKER_IMAGE}:${DOCKER_TAG}