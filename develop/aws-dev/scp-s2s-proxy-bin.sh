#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &> /dev/null && pwd)
ROOT_DIR="$(realpath "${SCRIPT_DIR}/../..")"
BIN_PATH="$(realpath "${ROOT_DIR}/bins/s2s-proxy")"

(
    cd "$ROOT_DIR"
    rm -f "$BIN_PATH"
    GOOS=linux GOARCH=amd64 make s2s-proxy
)

# Need root to write to /usr/local/bin on the target.
# Copy to remote's home dir, and then 'sudo mv' it into place.
echo 'rm -f /home/ec2-user/s2s-proxy' | $SCRIPT_DIR/ssh.sh
$SCRIPT_DIR/scp.sh "$BIN_PATH" s2s-proxy
echo 'sudo mv s2s-proxy /usr/local/bin/' | $SCRIPT_DIR/ssh.sh
