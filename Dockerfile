ARG BASE_SERVER_IMAGE=temporalio/base-server:1.15.7

FROM ${BASE_SERVER_IMAGE} as base-server

WORKDIR /etc/temporal

ENV TEMPORAL_HOME=/etc/temporal

RUN addgroup -g 1000 temporal
RUN adduser -u 1000 -G temporal -D temporal
RUN mkdir /etc/temporal/config
RUN chown -R temporal:temporal /etc/temporal/config
USER temporal

# binaries
COPY ./build/linux/amd64/s2s-proxy /usr/local/bin
