#!/usr/bin/env bash

set -xeuo pipefail

# Simple check to avoid accidentally running this locally.
if ! grep 'ec2' <(uname -a); then
    echo 'Only run this on the remote instance'
    exit 1
fi

# https://learn.temporal.io/tutorials/infrastructure/nginx-sqlite-binary/

# Some temporal cluster config fields
CLUSTER_NAME="pglass-s2c"
# MUST match all other connected clusters.
FAILOVER_VERSION_INCREMENT="1000000"
# Must NOT match any other connected clusters.
INITIAL_FAILOVER_VERSION="11011"

PROXY_OUTBOUND_LISTEN_ADDRESS='0.0.0.0:5333'
PROXY_INBOUND_LISTEN_ADDRESS='0.0.0.0:7233'
PROXY_OUTBOUND_FORWARD_ADDRESS='replication.s-cgs-ue1-l.cluster.tmprl-test.cloud:7233'

CERTS_DIR='/etc/temporal/certs'

cat << EOF | sudo tee /etc/temporal/temporal-server.yaml
log:
  stdout: true
  level: info

persistence:
  defaultStore: sqlite-default
  visibilityStore: sqlite-visibility
  numHistoryShards: 4
  datastores:
    sqlite-default:
      sql:
        pluginName: "sqlite"
        databaseName: "/etc/temporal/default.db"
        connectAddr: "localhost"
        connectProtocol: "tcp"
        connectAttributes:
          cache: "private"
          setup: true
        tls:
          enabled: false
          caFile: ""
          certFile: ""
          keyFile: ""
          enableHostVerification: false
          serverName: ""

    sqlite-visibility:
      sql:
        pluginName: "sqlite"
        databaseName: "/etc/temporal/visibility.db"
        connectAddr: "localhost"
        connectProtocol: "tcp"
        connectAttributes:
          cache: "private"
          setup: true
        tls:
          enabled: false
          caFile: ""
          certFile: ""
          keyFile: ""
          enableHostVerification: false
          serverName: ""


global:
  membership:
    maxJoinDuration: 30s
    broadcastAddress: "127.0.0.1"
  pprof:
    port: 7936
  metrics:
    prometheus:
      framework: "tally"
      timerType: "histogram"
      listenAddress: "127.0.0.1:8000"

services:
  frontend:
    rpc:
      grpcPort: 17233
      membershipPort: 6933
      bindOnLocalHost: false
      bindOnIP: 0.0.0.0
      httpPort: 7243

  matching:
    rpc:
      grpcPort: 7235
      membershipPort: 6935
      bindOnLocalHost: true

  history:
    rpc:
      grpcPort: 7234
      membershipPort: 6934
      bindOnLocalHost: true

  worker:
    rpc:
      grpcPort: 7239
      membershipPort: 6939
      bindOnLocalHost: true

clusterMetadata:
  enableGlobalNamespace: true
  failoverVersionIncrement: $FAILOVER_VERSION_INCREMENT
  masterClusterName: "$CLUSTER_NAME"
  currentClusterName: "$CLUSTER_NAME"
  clusterInformation:
    $CLUSTER_NAME:
      enabled: true
      initialFailoverVersion: $INITIAL_FAILOVER_VERSION
      rpcName: "frontend"
      rpcAddress: "localhost:17233"
      httpAddress: "localhost:7243"

dcRedirectionPolicy:
  policy: "all-apis-forwarding"

dynamicConfigClient:
  filepath: "/etc/temporal/dynamic-config.yaml"
  pollInterval: "10s"
EOF

cat << EOF | sudo tee /etc/temporal/dynamic-config.yaml
system.enableEagerWorkflowStart:
  - value: true
limit.maxIDLength:
  - value: 255
    constraints: {}
frontend.workerVersioningDataAPIs:
  - value: true
frontend.workerVersioningRuleAPIs:
  - value: true
frontend.workerVersioningWorkflowAPIs:
  - value: true
system.enableNexus:
  - value: false
component.nexusoperations.callback.endpoint.template:
  - value: http://localhost:7243/namespaces/{{.NamespaceName}}/nexus/callback
component.callbacks.allowedAddresses:
  - value:
      - Pattern: "*"
        AllowInsecure: true
matching.queryWorkflowTaskTimeoutLogRate:
  - value: 1.0
history.ReplicationEnableUpdateWithNewTaskMerge:
  - value: true
history.enableWorkflowExecutionTimeoutTimer:
  - value: true
history.hostLevelCacheMaxSize:
  - value: 8192
EOF

cat << EOF | sudo tee /etc/temporal/temporal-ui-server.yaml
temporalGrpcAddress: 127.0.0.1:17233
host: 127.0.0.1
port: 8233
enableUi: true
cors:
  allowOrigins:
    - http://localhost:8233
defaultNamespace: default
EOF

cat << EOF | sudo tee /etc/systemd/system/temporal.service
[Unit]
Description=Temporal Service
After=network.target

[Service]
User=temporal
Group=temporal
ExecStart=temporal-server -r / -c etc/temporal/ -e temporal-server start

[Install]
WantedBy=multi-user.target
EOF

cat << EOF | sudo tee /etc/systemd/system/temporal-ui.service
[Unit]
Description=Temporal UI Server
After=network.target

[Service]
User=temporal
Group=temporal
ExecStart=temporal-ui-server -r / -c etc/temporal/ -e temporal-ui-server start

[Install]
WantedBy=multi-user.target
EOF

cat << EOF | sudo tee /etc/systemd/system/temporal-s2s-proxy.service
[Unit]
Description=Temporal S2S Proxy
After=network.target

[Service]
User=temporal
Group=temporal
ExecStart=s2s-proxy start --config /etc/temporal/s2s-proxy.yaml

[Install]
WantedBy=multi-user.target
EOF

cat << 'EOF' | sudo tee /etc/nginx/conf.d/temporal-ui.conf
server {
    listen 80 default_server;
    listen [::]:80 default_server;

    access_log /var/log/nginx/temporal.access.log;
    error_log /var/log/nginx/temporal.error.log;

    location / {
        proxy_pass http://127.0.0.1:8233;
        proxy_http_version 1.1;
        proxy_read_timeout 300;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $http_host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Real-PORT $remote_port;
        allow all;
    }
}
EOF

cat << EOF | sudo tee /etc/temporal/s2s-proxy.yaml
inbound:
  name: "inbound-server"
  server:
    listenAddress: $PROXY_INBOUND_LISTEN_ADDRESS
    tls:
      certificatePath: "$CERTS_DIR/tls.crt"
      keyPath: "$CERTS_DIR/tls.key"
      clientCAPath: "$CERTS_DIR/ca.crt"
      requireClientAuth: false
  client:
    forwardAddress: localhost:17233
  namespaceNameTranslation:
    mappings:
      - localName: "pglass-s2c-01"
        remoteName: "pglass-s2c-01.temporal-dev"
      - localName: "pglass-s2c-02"
        remoteName: "pglass-s2c-02.temporal-dev"
      - localName: "pglass-s2c-03"
        remoteName: "pglass-s2c-03.temporal-dev"

outbound:
  name: "outbound-server"
  server:
    listenAddress: $PROXY_OUTBOUND_LISTEN_ADDRESS
  client:
    forwardAddress: $PROXY_OUTBOUND_FORWARD_ADDRESS
    tls:
      certificatePath: "$CERTS_DIR/tls.crt"
      keyPath: "$CERTS_DIR/tls.key"
      serverCAPath: "$CERTS_DIR/ca.crt"
      serverName: ""
  namespaceNameTranslation:
    mappings:
      - localName: "pglass-s2c-01"
        remoteName: "pglass-s2c-01.temporal-dev"
      - localName: "pglass-s2c-02"
        remoteName: "pglass-s2c-02.temporal-dev"
      - localName: "pglass-s2c-03"
        remoteName: "pglass-s2c-03.temporal-dev"

EOF

# Restart services so configs take effect.
sudo systemctl daemon-reload

sudo systemctl restart temporal
sudo systemctl enable temporal

sudo systemctl restart temporal-ui
sudo systemctl enable temporal-ui

sudo systemctl restart temporal-s2s-proxy
sudo systemctl enable temporal-s2s-proxy

sudo systemctl restart nginx
