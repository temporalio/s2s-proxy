#!/usr/bin/env bash

set -euo pipefail

set -x

TARGET_PKG="common/proto/1_22"
rm -rf "./$TARGET_PKG"
mkdir -p "$TARGET_PKG/api" "$TARGET_PKG/server"

COMMON_DIR=$(realpath ./common)
TARGET_DIR=$(realpath "./$TARGET_PKG")

# Copy all of the api-go pkg
API_DIR="${TARGET_DIR}/api"
rm -rf "$API_DIR"
git clone https://github.com/temporalio/api-go.git "$API_DIR"
(
    cd "$API_DIR"
    git reset --hard bb03061759c82712a4b933f5175834baebee9c9a

)
rm -rf \
    "$API_DIR/.github" \
    "$API_DIR/cmd" \
    "$API_DIR/internal/temporalgateway" \
    "$API_DIR/proxy/marshal.go"
find "$API_DIR" -type f -maxdepth 1 -delete
find "$API_DIR" -type f -name '*test.go' -delete
find "$API_DIR" -type f -name '*.pb.gw.go' -delete
find "$API_DIR" -type d -name '*mock' -exec rm -r '{}' '+'
# Fix import path
find "$API_DIR" -type f -name '*.go' | \
    xargs -n 1 sed -i '' "s;go.temporal.io/api;github.com/temporalio/s2s-proxy/${TARGET_PKG}/api;g"

SERVER_DIR="${COMMON_DIR}/.tmp-server"
if [ ! -d "$SERVER_DIR" ]; then
    git clone https://github.com/temporalio/temporal.git "$SERVER_DIR"
fi
(
    cd "$SERVER_DIR"
    git fetch --tags
    git reset --hard v1.22.2
)

for pkg in \
    api/adminservice \
    api/cluster \
    api/clock \
    api/enums \
    api/history \
    api/namespace \
    api/persistence \
    api/replication \
    api/taskqueue \
    api/update \
    api/workflow \
    common/constants.go \
    common/debug/debug.go \
    common/util \
    common/persistence/serialization \
    common/definition \
    common/primitives/timestamp \
    common/codec/jsonpb.go \
    common/tasks \
    common/log \
    common/quotas \
    common/primitives/role.go \
    common/backoff/retrypolicy.go \
    service/history/tasks \
; do
    SRC="$SERVER_DIR/${pkg}"
    DEST="${TARGET_DIR}/server/${pkg}"
    mkdir -p "$(dirname $DEST)"
    cp -R "$SRC" "$DEST"

    # Fix import paths
    find "$DEST" -type f | \
        xargs -n 1 sed -i '' "s;go.temporal.io/server;github.com/temporalio/s2s-proxy/${TARGET_PKG}/server;g"
    find "$DEST" -type f | \
        xargs -n 1 sed -i '' "s;go.temporal.io/api;github.com/temporalio/s2s-proxy/${TARGET_PKG}/api;g"

    # Remove '//go:build TEMPORAL_DEBUG' build constraint
    # ???
    find "$DEST" -type f | \
        xargs -n 1 sed -i '' "s;//.*TEMPORAL_DEBUG;;g"

    find "$TARGET_DIR" -type f -name '*test.go' -delete
done

# Unnecessary files
rm -f "${TARGET_DIR}/api/workflowservice/v1/service.pb.gw.go"

rm -f "${TARGET_DIR}/server/service/history/tasks/predicates.go"
rm -f "${TARGET_DIR}/server/common/quotas/delayed_request_rate_limiter.go"
rm -f "${TARGET_DIR}/server/common/quotas/delayed_request_rate_limiter_test.go"
rm -f "${TARGET_DIR}/server/common/tasks/fifo_scheduler.go"
rm -f "${TARGET_DIR}/server/common/tasks/interleaved_weighted_round_robin.go"
rm -f "${TARGET_DIR}/server/common/tasks/sequential_scheduler.go"

go mod tidy
make fmt

#rm -rf "$API_DIR"
rm -rf "$SERVER_DIR"
