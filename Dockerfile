ARG BASE_IMAGE=temporalio/base-server:1.15.7
ARG ADMIN_TOOLS_IMAGE=temporalio/admin-tools:1.22

FROM ${BASE_IMAGE} AS base

# Install tools
RUN apk add --update --no-cache bash ca-certificates openssh jq

# Admin tools setup
FROM ${ADMIN_TOOLS_IMAGE} AS temporal-admin-tools

WORKDIR /etc/s2s-proxy

COPY ./build/linux/amd64/s2s-proxy /usr/local/bin
COPY scripts/entrypoint.sh /opt/entrypoint.sh
COPY scripts/start.sh /opt/start.sh

ENTRYPOINT ["/opt/entrypoint.sh"]
CMD ["/opt/start.sh"]
