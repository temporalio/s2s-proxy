# Build stage
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

ARG TARGETARCH

# System dependencies
RUN apk add --no-cache \
    ca-certificates \
    git \
    make \
    openssh

# Making sure that dependency is not touched
ENV GOFLAGS="-mod=readonly"

WORKDIR /s2s-proxy

# Dependency manifests
COPY go.mod go.sum ./

RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Source code
COPY . .

# Build
# need to make clean first in case binaries to be built are stale
RUN make clean && CGO_ENABLED=0 make bins

# Runtime stage
FROM alpine:3.22 AS base

COPY --from=builder /s2s-proxy/bins/s2s-proxy /usr/local/bin/
COPY --from=builder /s2s-proxy/scripts/entrypoint.sh /opt/entrypoint.sh
COPY --from=builder /s2s-proxy/scripts/start.sh /opt/start.sh

ENTRYPOINT ["/opt/entrypoint.sh"]
CMD ["/opt/start.sh"]
