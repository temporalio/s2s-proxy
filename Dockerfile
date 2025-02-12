# Stage 1: Build
FROM --platform=$BUILDPLATFORM temporalio/base-builder:1.15.3 AS builder

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

# Add temporal admin tools
FROM temporalio/admin-tools:1.22 AS temporal-admin-tools

# Stage 2: Create image

FROM alpine:3.17 AS base

# Install tools
RUN apk add bash ca-certificates openssh jq

COPY --from=builder /s2s-proxy/bins/s2s-proxy /usr/local/bin/
COPY --from=builder /s2s-proxy/scripts/entrypoint.sh /opt/entrypoint.sh
COPY --from=builder /s2s-proxy/scripts/start.sh /opt/start.sh
COPY --from=temporal-admin-tools /usr/local/bin/tctl /usr/local/bin/tctl
COPY --from=temporal-admin-tools /usr/local/bin/temporal /usr/local/bin/temporal
COPY --from=temporal-admin-tools /usr/local/bin/tdbg /usr/local/bin/tdbg

ENTRYPOINT ["/opt/entrypoint.sh"]
CMD ["/opt/start.sh"]
