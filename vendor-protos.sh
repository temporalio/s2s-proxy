#!/usr/bin/env bash

set -euo pipefail

set -x

TARGET_PKG="common/proto/1_22"
rm -rf "./$TARGET_PKG"
mkdir -p "$TARGET_PKG/api" "$TARGET_PKG/server"

COMMON_DIR=$(realpath ./common)
TARGET_DIR=$(realpath "./$TARGET_PKG")

API_DIR="${COMMON_DIR}/.tmp-api"
SERVER_DIR="${COMMON_DIR}/.tmp-server"
if [ ! -d "$API_DIR" ]; then
    git clone https://github.com/temporalio/api-go.git "$API_DIR"
fi
(
    cd "$API_DIR"
    git reset --hard bb03061759c82712a4b933f5175834baebee9c9a
)

if [ ! -d "$SERVER_DIR" ]; then
    git clone https://github.com/temporalio/temporal.git "$SERVER_DIR"
fi
(
    cd "$SERVER_DIR"
    git fetch --tags
    git reset --hard v1.22.2
)

for pkg in \
    common \
    enums \
    errordetails \
    failure \
    history \
    internal/temporaljsonpb \
    namespace \
    replication \
    sdk \
    serviceerror \
    taskqueue \
    update \
    version \
    workflow \
; do
    SRC="$API_DIR/${pkg}"
    DEST="${TARGET_DIR}/api/${pkg}"
    mkdir -p "$(dirname $DEST)"
    cp -R "$SRC" "$DEST"

    # Fix import paths
    find "$DEST" -type f | \
        xargs -n 1 sed -i '' "s;go.temporal.io/api;github.com/temporalio/s2s-proxy/${TARGET_PKG}/api;g"
done

for pkg in \
    api/persistence \
    api/clock \
    api/enums \
    api/history \
    api/namespace \
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

    # Unnecessary files
    rm -f "${TARGET_DIR}/server/service/history/tasks/predicates.go"
    rm -f "${TARGET_DIR}/server/common/quotas/delayed_request_rate_limiter.go"
    rm -f "${TARGET_DIR}/server/common/quotas/delayed_request_rate_limiter_test.go"
    rm -f "${TARGET_DIR}/server/common/tasks/fifo_scheduler.go"
    rm -f "${TARGET_DIR}/server/common/tasks/interleaved_weighted_round_robin.go"
    rm -f "${TARGET_DIR}/server/common/tasks/sequential_scheduler.go"

    find "$TARGET_DIR" -type f -name '*test.go' -delete
done

go mod tidy
make fmt

rm -rf "$API_DIR"
rm -rf "$SERVER_DIR"
