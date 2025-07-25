# Stage 1: Build
FROM --platform=$BUILDPLATFORM temporalio/base-builder:1.15.5 AS builder

ARG TARGETARCH

# Install build tools
RUN apk add --update --no-cache ca-certificates git make openssh

# Making sure that dependency is not touched
ENV GOFLAGS="-mod=readonly"

WORKDIR /s2s-proxy

# Copy go mod dependencies and build cache
COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . .

# need to make clean first in case binaries to be built are stale
RUN make clean && CGO_ENABLED=0 make bins

# Stage 2: Create image
FROM alpine:3.17 AS base

COPY --from=builder /s2s-proxy/bins/s2s-proxy /usr/local/bin/
COPY --from=builder /s2s-proxy/scripts/entrypoint.sh /opt/entrypoint.sh
COPY --from=builder /s2s-proxy/scripts/start.sh /opt/start.sh

ENTRYPOINT ["/opt/entrypoint.sh"]
CMD ["/opt/start.sh"]
