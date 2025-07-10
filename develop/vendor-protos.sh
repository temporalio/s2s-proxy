#!/usr/bin/env bash

set -euo pipefail

TARGET_PKG="proto/1_22"
mkdir -p "./$TARGET_PKG"
PROTO_DIR=$(realpath ./proto)
TARGET_DIR=$(realpath "./$TARGET_PKG")

API_DIR="${TARGET_DIR}/api"
API_TMP_DIR="${PROTO_DIR}/.tmp-api"
API_GIT_REPO=temporalio/api-go
API_GIT_TAG=bb03061759c82712a4b933f5175834baebee9c9a

SERVER_DIR="${TARGET_DIR}/server"
SERVER_TMP_DIR="${PROTO_DIR}/.tmp-server"
SERVER_GIT_REPO=temporalio/temporal
SERVER_GIT_TAG=v1.22.2

delete_code() {
    local file="$1"
    shift
    echo "Edit $file"
    for pattern in "$@"; do
        comby "$pattern" '' -in-place "$file"
    done

    # Run goimports to remove any now-unused imports
    goimports -w "$file"
}

include_file() {
    local tmp_dir="$1"
    local src="$2"
    local target_dir="$3"

    local src="$(echo "$src" | sed "s;$tmp_dir/;;")"
    local dest="${target_dir}/$(echo "$src" | sed "s;$tmp_dir/;;")"
    echo "Include $dest"
    mkdir -p "$(dirname $dest)"
    cp -R "$tmp_dir/$src" $dest
}

clone_repo() {
    local repo="https://github.com/${1}.git"
    local commit="$2"
    local tmp_dir="$3"

    echo "Clone $repo"
    if [ ! -d "$tmp_dir" ]; then
        git clone "$repo" "$tmp_dir"
    fi
    cd "$tmp_dir"
    git reset --hard $commit
}

write_readme() {
    local repo="$1"
    local commit="$2"
    local target_dir="$3"

    local tree_link="https://github.com/${repo}/tree/${commit}"
    local out_file="${target_dir}/README.md"
    cat <<EOF > "$out_file"
# Overview

This is a partial copy of [${repo} at ${commit}](${tree_link})

Temporal v1.22 used gogo-protobuf for serialization, which allowed it to store
invalid UTF8 provided from the SDK and workers locally. Temporal 1.23 switched
to google-protobuf, which requires all string data be valid UTF8. The proxy uses
these proto definitions to seamlessly repair broken UTF8 strings in flight
without dropping any of the valid data.
EOF
}

# Copy files from temporalio/api-go
(
    clone_repo temporalio/api-go "$API_GIT_TAG" "$API_TMP_DIR"
    rm -rf "$API_DIR"
    for SRC in $(find "$API_TMP_DIR" -type f -name '*.pb.go'); do
        include_file "$API_TMP_DIR" "$SRC" "$API_DIR"
    done
    include_file "$API_TMP_DIR" "serviceerror" "$API_DIR"

    # Fix import path
    find "$API_DIR" -type f -name '*.go' | \
        xargs -n 1 sed -i '' "s;go.temporal.io/api;github.com/temporalio/s2s-proxy/${TARGET_PKG}/api;g"
)

# Copy/edit files from temporalio/temporal
(
    clone_repo temporalio/temporal "$SERVER_GIT_TAG" "$SERVER_TMP_DIR"
    rm -rf "$SERVER_DIR"

    # Copy all protobuf files
    for SRC in $(find "$SERVER_TMP_DIR" -type f -name '*.pb.go'); do
        include_file "$SERVER_TMP_DIR" "$SRC" "$SERVER_DIR"
    done

    # Copy common/persistence/serialization + dependendent packages
    for pkg in \
        common/persistence/serialization \
        common/codec/jsonpb.go \
        common/constants.go \
        common/definition/workflow_key.go \
        common/primitives/timestamp/time.go \
        service/history/tasks \
    ; do
        include_file "$SERVER_TMP_DIR" "$pkg" "$SERVER_DIR"
    done

    # Remove some known unneeded files
    rm -f "$SERVER_DIR/service/history/tasks/predicates.go"

    # Fix import paths
    find "$SERVER_DIR" -type f | xargs -n 1 sed -i '' "s;go.temporal.io/server;github.com/temporalio/s2s-proxy/${TARGET_PKG}/server;g"
    find "$SERVER_DIR" -type f | xargs -n 1 sed -i '' "s;go.temporal.io/api;github.com/temporalio/s2s-proxy/${TARGET_PKG}/api;g"

    # Surgical code edits to remove specific code we don't need.
    # This reduces the number of files we need to pull in.
    delete_code "$SERVER_DIR/service/history/tasks/utils.go" \
        'func InitializeLogger(:[args]) :[rets] { :[code] }' \
        'func Tags(:[args]) :[rets] { :[code] }'

    delete_code "$SERVER_DIR/common/primitives/timestamp/time.go" \
        'func MinDurationPtr(:[args]) :[rets] { :[code] }' \

    delete_code "$SERVER_DIR/common/constants.go" \
        'DefaultWorkflowTaskTimeout = :[var]'

    delete_code "$SERVER_DIR/service/history/tasks/workflow_task_timer.go" \
        'func (d *WorkflowTaskTimeoutTask) Cancel() { :[code] }' \
        'func (d *WorkflowTaskTimeoutTask) State() :[rets] { :[code] }'
)

# Remove non-go files and test files
find "$TARGET_DIR" -type f -not -name '*.go' -delete
find "$TARGET_DIR" -type f -name '*test.go' -delete
find "$TARGET_DIR" -type f -name '*mock.go' -delete
# Remove empty directories
find "$TARGET_DIR" -type d -empty -delete

find "$API_DIR" -type f -name '*.go' | xargs -n1 sed -i '' "1 i\\
// Copied from https://github.com/${API_GIT_REPO}/tree/${API_GIT_TAG}. DO NOT EDIT.\\

"
find "$SERVER_DIR" -type f -name '*.go' | xargs -n1 sed -i '' "1 i\\
// Copied from https://github.com/${SERVER_GIT_REPO}/tree/${SERVER_GIT_TAG}. DO NOT EDIT.\\

"

write_readme "$API_GIT_REPO" "$API_GIT_TAG" "$API_DIR"
write_readme "$SERVER_GIT_REPO" "$SERVER_GIT_TAG" "$SERVER_DIR"

go mod tidy
make fmt
make lint

rm -rf "$API_TMP_DIR"
rm -rf "$SERVER_TMP_DIR"
