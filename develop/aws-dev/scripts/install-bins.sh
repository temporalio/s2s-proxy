#!/usr/bin/env bash

set -xeuo pipefail

# Simple check to avoid accidentally running this locally.
if ! grep 'ec2' <(uname -a); then
    echo 'Only run this on the remote instance'
    exit 1
fi

# https://learn.temporal.io/tutorials/infrastructure/nginx-sqlite-binary/

OSS_VERSION="1.25.0"
OSS_FILENAME="temporal_${OSS_VERSION}_linux_amd64.tar.gz"
OSS_RELEASE_URL="https://github.com/temporalio/temporal/releases/download/v${OSS_VERSION}/${OSS_FILENAME}"

UI_VERSION="2.30.3"
UI_FILENAME="ui-server_${UI_VERSION}_linux_amd64.tar.gz"
UI_RELEASE_URL="https://github.com/temporalio/ui-server/releases/download/v${UI_VERSION}/${UI_FILENAME}"

CLI_VERSION="1.0.0"
CLI_FILENAME="temporal_cli_${CLI_VERSION}_linux_amd64.tar.gz"
CLI_RELEASE_URL="https://github.com/temporalio/cli/releases/download/v${CLI_VERSION}/${CLI_FILENAME}"

TCTL_VERSION="1.18.1"
TCTL_FILENAME="tctl_${TCTL_VERSION}_linux_amd64.tar.gz"
TCTL_RELEASE_URL="https://github.com/temporalio/tctl/releases/download/v${TCTL_VERSION}/${TCTL_FILENAME}"




# nc - netcat for debugging
sudo dnf install -y nginx nc

curl -OL "$OSS_RELEASE_URL"
tar -xzf "$OSS_FILENAME"
sudo mv temporal-server /usr/bin/temporal-server
sudo chmod +x /usr/bin/temporal-server

curl -OL "$UI_RELEASE_URL"
tar -xzf "$UI_FILENAME"
sudo mv ui-server /usr/bin/temporal-ui-server
sudo chmod +x /usr/bin/temporal-ui-server

curl -OL "$CLI_RELEASE_URL"
tar -xzf "$CLI_FILENAME"
sudo mv temporal /usr/bin/temporal
sudo chmod +x /usr/bin/temporal

curl -OL "$TCTL_RELEASE_URL"
tar -xzf "$TCTL_FILENAME"
sudo mv tctl /usr/bin/tctl
sudo chmod +x /usr/bin/tctl

sudo useradd temporal || true
sudo mkdir -p /etc/temporal
sudo chown temporal /etc/temporal
