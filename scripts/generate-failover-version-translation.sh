#!/usr/bin/env bash

set -euo pipefail

usage() {
  printf '%s\n' "usage: $0 --source-profile PROFILE --target-profile PROFILE --namespace NAME [options]" >&2
  printf '%s\n' "  --source-address ADDRESS --target-address ADDRESS --target-namespace NAME" >&2
  printf '%s\n' "  --historical-source-ifv LANE (repeatable) --temporal-bin PATH" >&2
  exit 2
}

source_profile=
target_profile=
source_address=
target_address=
namespace=
target_namespace=
temporal_bin=temporal
historical_source_ifvs=()

while (($#)); do
  case "$1" in
    --source-profile) (($# >= 2)) || usage; source_profile=$2; shift 2 ;;
    --target-profile) (($# >= 2)) || usage; target_profile=$2; shift 2 ;;
    --source-address) (($# >= 2)) || usage; source_address=$2; shift 2 ;;
    --target-address) (($# >= 2)) || usage; target_address=$2; shift 2 ;;
    --namespace) (($# >= 2)) || usage; namespace=$2; shift 2 ;;
    --target-namespace) (($# >= 2)) || usage; target_namespace=$2; shift 2 ;;
    --historical-source-ifv) (($# >= 2)) || usage; historical_source_ifvs+=("$2"); shift 2 ;;
    --temporal-bin) (($# >= 2)) || usage; temporal_bin=$2; shift 2 ;;
    -h|--help) usage ;;
    *) usage ;;
  esac
done

[[ -n "$source_profile" && -n "$target_profile" && -n "$namespace" ]] || usage
[[ -n "$target_namespace" ]] || target_namespace=$namespace
command -v "$temporal_bin" >/dev/null 2>&1 || { printf 'temporal binary not found: %s\n' "$temporal_bin" >&2; exit 1; }
command -v jq >/dev/null 2>&1 || { printf 'jq is required\n' >&2; exit 1; }
command -v sort >/dev/null 2>&1 || { printf 'sort is required\n' >&2; exit 1; }

work_dir=$(mktemp -d "${TMPDIR:-/tmp}/s2s-failover-version.XXXXXX")
trap 'status=$?; rm -rf "$work_dir"; exit "$status"' EXIT
trap 'printf "generator command failed at line %s\n" "$LINENO" >&2' ERR

run_temporal() {
  local profile=$1 address=$2 output=$3
  shift 3
  local args=(--profile "$profile" --grpc-meta s2s-request-translation=false)
  if [[ -n "$address" ]]; then
    args+=(--address "$address")
  fi
  "$temporal_bin" "${args[@]}" "$@" -o json >"$output"
}

run_temporal "$source_profile" "$source_address" "$work_dir/source-cluster.json" operator cluster describe --detail
run_temporal "$source_profile" "$source_address" "$work_dir/source-clusters.json" operator cluster list
run_temporal "$source_profile" "$source_address" "$work_dir/source-namespace.json" operator namespace describe --namespace "$namespace"
run_temporal "$target_profile" "$target_address" "$work_dir/target-cluster.json" operator cluster describe --detail
run_temporal "$target_profile" "$target_address" "$work_dir/target-clusters.json" operator cluster list

if run_temporal "$target_profile" "$target_address" "$work_dir/target-namespace.json" operator namespace describe --namespace "$target_namespace" 2>"$work_dir/target-namespace.err"; then
  printf 'target namespace %s already exists; refusing to generate a mapping\n' "$target_namespace" >&2
  exit 1
elif ! grep -Eiq 'not[ -]?found|NamespaceNotFound' "$work_dir/target-namespace.err"; then
  printf 'failed to verify that target namespace %s is absent\n' "$target_namespace" >&2
  sed -n '1,5p' "$work_dir/target-namespace.err" >&2
  exit 1
fi

jq_string() {
  local expression=$1 file=$2
  jq -er "$expression | if type == \"string\" then . else tostring end" "$file"
}

source_name=$(jq_string '(.clusterName // .cluster_name)' "$work_dir/source-cluster.json")
target_name=$(jq_string '(.clusterName // .cluster_name)' "$work_dir/target-cluster.json")
source_uuid=$(jq_string '(.clusterId // .cluster_id // "unknown")' "$work_dir/source-cluster.json")
target_uuid=$(jq_string '(.clusterId // .cluster_id // "unknown")' "$work_dir/target-cluster.json")
source_fvi=$(jq_string '(.failoverVersionIncrement // .failover_version_increment)' "$work_dir/source-cluster.json")
target_fvi=$(jq_string '(.failoverVersionIncrement // .failover_version_increment)' "$work_dir/target-cluster.json")
source_ifv=$(jq_string '(.initialFailoverVersion // .initial_failover_version)' "$work_dir/source-cluster.json")
target_ifv=$(jq_string '(.initialFailoverVersion // .initial_failover_version)' "$work_dir/target-cluster.json")
cluster_global_setting() {
  jq -er '
    if has("isGlobalNamespaceEnabled") then .isGlobalNamespaceEnabled
    elif has("is_global_namespace_enabled") then .is_global_namespace_enabled
    else "unavailable"
    end
  ' "$1"
}

source_global=$(cluster_global_setting "$work_dir/source-cluster.json")
target_global=$(cluster_global_setting "$work_dir/target-cluster.json")
namespace_global=$(jq -er '(.isGlobalNamespace // .is_global_namespace // false)' "$work_dir/source-namespace.json")
active_cluster=$(jq_string '(.replicationConfig.activeClusterName // .replication_config.active_cluster_name // .activeClusterName // .active_cluster_name)' "$work_dir/source-namespace.json")
namespace_id=$(jq_string '(.namespaceInfo.id // .namespace_info.id // .namespaceId // .namespace_id)' "$work_dir/source-namespace.json")
namespace_version=$(jq_string '(.failoverVersion // .failover_version)' "$work_dir/source-namespace.json")

is_positive_int64() {
  local value=$1 normalized
  [[ "$value" =~ ^[0-9]+$ ]] || return 1
  normalized=${value#${value%%[!0]*}}
  [[ -n "$normalized" ]] || normalized=0
  ((${#normalized} < 19)) && ((10#$normalized > 0)) && return 0
  ((${#normalized} == 19)) && [[ "$normalized" < 9223372036854775808 || "$normalized" == 9223372036854775807 ]]
}

for value in "$source_fvi" "$target_fvi" "$source_ifv" "$target_ifv" "$namespace_version"; do
  is_positive_int64 "$value" || { printf 'invalid positive signed-64-bit Temporal value\n' >&2; exit 1; }
done
((source_ifv < source_fvi && target_ifv < target_fvi)) || { printf 'endpoint IFVs must be below their FVIs\n' >&2; exit 1; }
[[ "$source_global" != false && "$target_global" != false ]] || { printf 'both clusters must enable global namespaces\n' >&2; exit 1; }
if [[ "$source_global" == unavailable || "$target_global" == unavailable ]]; then
  printf '%s\n' 'warning: temporal operator cluster describe does not expose the global-namespace setting; validating the source through its global namespace instead' >&2
fi
[[ "$namespace_global" == true ]] || { printf 'source namespace must be global\n' >&2; exit 1; }
[[ "$active_cluster" == "$source_name" ]] || { printf 'source namespace must be active on %s\n' "$source_name" >&2; exit 1; }
[[ "$source_name" != "$target_name" && "$source_uuid" != "$target_uuid" ]] || { printf 'source and target must be different clusters\n' >&2; exit 1; }

jq -er '(.replicationConfig.clusters // .replication_config.clusters // [])[] | (.clusterName // .cluster_name)' \
  "$work_dir/source-namespace.json" >"$work_dir/namespace-replicas" || true
replica_count=$(wc -l <"$work_dir/namespace-replicas" | tr -d ' ')
[[ "$replica_count" == 1 && "$(sed -n '1p' "$work_dir/namespace-replicas")" == "$source_name" ]] || {
  printf 'source namespace must have only the active source cluster as a current replica\n' >&2
  exit 1
}

cluster_rows() {
  jq -r '(if type == "array" then .
    elif (.clusters | type) == "array" then .clusters
    elif (.clusterMetadata | type) == "array" then .clusterMetadata
    elif (.cluster_metadata | type) == "array" then .cluster_metadata
    else [] end)[] |
    [(.clusterName // .cluster_name // ""),
     ((.initialFailoverVersion // .initial_failover_version // 0)|tostring),
     (.clusterId // .cluster_id // "unknown")] | @tsv' "$1"
}
cluster_rows "$work_dir/source-clusters.json" >"$work_dir/source-clusters.tsv"
cluster_rows "$work_dir/target-clusters.json" >"$work_dir/target-clusters.tsv"

printf '%s\n' "$source_ifv" >"$work_dir/required-source-lanes"
printf '%s\n' "$namespace_version" >"$work_dir/observed-versions"
jq -r '(.failoverHistory // .failover_history // [])[] | (.failoverVersion // .failover_version) | tostring' \
  "$work_dir/source-namespace.json" >>"$work_dir/observed-versions"

canonical_lane() {
  local version=$1 fvi=$2
  printf '%d\n' "$(( (version - 1) % fvi + 1 ))"
}
while IFS= read -r version; do
  is_positive_int64 "$version" || { printf 'invalid failover-history version\n' >&2; exit 1; }
  canonical_lane "$version" "$source_fvi" >>"$work_dir/required-source-lanes"
done <"$work_dir/observed-versions"
for ((history_index=0; history_index<${#historical_source_ifvs[@]}; history_index++)); do
  lane=${historical_source_ifvs[$history_index]}
  is_positive_int64 "$lane" && ((lane <= source_fvi)) || { printf 'invalid --historical-source-ifv\n' >&2; exit 1; }
  printf '%s\n' "$lane" >>"$work_dir/required-source-lanes"
done
sort -n -u "$work_dir/required-source-lanes" -o "$work_dir/required-source-lanes"

: >"$work_dir/fixed.tsv"
: >"$work_dir/occupied-source"
: >"$work_dir/occupied-target"
cut -f2 "$work_dir/source-clusters.tsv" | sed '/^0$/d' | sort -n -u >"$work_dir/occupied-source"
cut -f2 "$work_dir/target-clusters.tsv" | sed '/^0$/d' | sort -n -u >"$work_dir/occupied-target"
while IFS=$'\t' read -r name local_lane old_uuid; do
  [[ -n "$name" && "$local_lane" != 0 ]] || continue
  target_row=$(awk -F '\t' -v name="$name" '$1 == name {print; exit}' "$work_dir/target-clusters.tsv")
  [[ -n "$target_row" ]] || continue
  remote_lane=$(printf '%s\n' "$target_row" | cut -f2)
  new_uuid=$(printf '%s\n' "$target_row" | cut -f3)
  printf '%s\t%s\n' "$local_lane" "$remote_lane" >>"$work_dir/fixed.tsv"
  printf '%s\n' "$local_lane" >>"$work_dir/required-source-lanes"
  if [[ "$name" == "$target_name" && "$old_uuid" != "$new_uuid" ]]; then
    printf 'warning: target cluster name %s was recreated (%s -> %s)\n' "$name" "$old_uuid" "$new_uuid" >&2
  fi
done <"$work_dir/source-clusters.tsv"
sort -n -u "$work_dir/required-source-lanes" -o "$work_dir/required-source-lanes"

target_source_lane=$(awk -F '\t' -v name="$target_name" '$1 == name {print $2; exit}' "$work_dir/source-clusters.tsv")
if [[ -n "$target_source_lane" ]]; then
  printf '%s\t%s\n' "$target_source_lane" "$target_ifv" >>"$work_dir/fixed.tsv"
  printf '%s\n' "$target_source_lane" >>"$work_dir/required-source-lanes"
fi

contains_line() { grep -qx "$1" "$2" 2>/dev/null; }
fixed_target() { awk -F '\t' -v lane="$1" '$1 == lane {print $2; exit}' "$work_dir/fixed.tsv"; }
fixed_source_for_target() { awk -F '\t' -v lane="$1" '$2 == lane {print $1; exit}' "$work_dir/fixed.tsv"; }

SOURCES=()
TARGETS=()
USED_TARGETS=()

target_usable() {
  local target=$1 source=$2 fixed_source
  ((target > 0 && target <= target_fvi)) || return 1
  local used
	if ((${#USED_TARGETS[@]})); then
		for used in "${USED_TARGETS[@]}"; do [[ "$used" == "$target" ]] && return 1; done
	fi
  if contains_line "$target" "$work_dir/occupied-target"; then
    fixed_source=$(fixed_source_for_target "$target")
    [[ "$fixed_source" == "$source" ]] || return 1
  fi
  return 0
}

allocate_from() {
  local index=$1 previous=$2 source fixed candidate next
  ((index == ${#SOURCES[@]})) && return 0
  source=${SOURCES[$index]}
  fixed=$(fixed_target "$source")
  if [[ -n "$fixed" ]]; then
    ((fixed > previous)) && target_usable "$fixed" "$source" || return 1
    TARGETS[$index]=$fixed
    USED_TARGETS+=("$fixed")
    allocate_from "$((index + 1))" "$fixed" && return 0
    unset "USED_TARGETS[$((${#USED_TARGETS[@]} - 1))]"
    return 1
  fi
	if ((source > previous)) && target_usable "$source" "$source"; then
		TARGETS[$index]=$source
		USED_TARGETS+=("$source")
		allocate_from "$((index + 1))" "$source" && return 0
		unset "USED_TARGETS[$((${#USED_TARGETS[@]} - 1))]"
	fi
	candidate=$((previous + 1))
	while ((candidate <= target_fvi)); do
		if ((candidate == source)); then candidate=$((candidate + 1)); continue; fi
		if target_usable "$candidate" "$source"; then
			TARGETS[$index]=$candidate
			USED_TARGETS+=("$candidate")
			allocate_from "$((index + 1))" "$candidate" && return 0
			unset "USED_TARGETS[$((${#USED_TARGETS[@]} - 1))]"
		fi
		candidate=$((candidate + 1))
	done
  return 1
}

try_allocate() {
  SOURCES=()
  TARGETS=()
  USED_TARGETS=()
  while IFS= read -r lane; do SOURCES+=("$lane"); done < <(sort -n -u "$work_dir/required-source-lanes")
  allocate_from 0 0
}

if [[ -z "$target_source_lane" ]]; then
  allocated_source=
	candidate=$target_ifv
	while ((candidate <= source_fvi)); do
		if ((candidate == target_ifv)); then next_candidate=1; else next_candidate=$((candidate + 1)); fi
		((next_candidate == target_ifv)) && next_candidate=$((next_candidate + 1))
		if contains_line "$candidate" "$work_dir/occupied-source" || contains_line "$candidate" "$work_dir/required-source-lanes"; then
			candidate=$next_candidate
			continue
		fi
    cp "$work_dir/required-source-lanes" "$work_dir/required-source-lanes.base"
    cp "$work_dir/fixed.tsv" "$work_dir/fixed.base.tsv"
    printf '%s\n' "$candidate" >>"$work_dir/required-source-lanes"
    printf '%s\t%s\n' "$candidate" "$target_ifv" >>"$work_dir/fixed.tsv"
    if try_allocate; then allocated_source=$candidate; break; fi
    mv "$work_dir/required-source-lanes.base" "$work_dir/required-source-lanes"
    mv "$work_dir/fixed.base.tsv" "$work_dir/fixed.tsv"
		candidate=$next_candidate
	done
  [[ -n "$allocated_source" ]] || { printf 'no complete order-preserving lane mapping exists\n' >&2; exit 1; }
  printf 'allocated virtual source lane %s for target cluster %s\n' "$allocated_source" "$target_name" >&2
elif ! try_allocate; then
  printf 'no complete order-preserving lane mapping exists\n' >&2
  exit 1
fi

: >"$work_dir/mapping.tsv"
for ((i=0; i<${#SOURCES[@]}; i++)); do printf '%s\t%s\n' "${SOURCES[$i]}" "${TARGETS[$i]}" >>"$work_dir/mapping.tsv"; done

translate_version() {
  local version=$1 input_fvi=$2 output_fvi=$3 direction=$4 epoch lane mapped
  ((version == 0)) && { printf '0\n'; return; }
  epoch=$(( (version - 1) / input_fvi ))
  lane=$(( (version - 1) % input_fvi + 1 ))
  if [[ "$direction" == forward ]]; then
    mapped=$(awk -F '\t' -v lane="$lane" '$1 == lane {print $2; exit}' "$work_dir/mapping.tsv")
  else
    mapped=$(awk -F '\t' -v lane="$lane" '$2 == lane {print $1; exit}' "$work_dir/mapping.tsv")
  fi
  [[ -n "$mapped" ]] || return 1
  ((epoch <= (9223372036854775807 - mapped) / output_fvi)) || return 1
  printf '%d\n' "$((epoch * output_fvi + mapped))"
}

while IFS= read -r version; do
  translated=$(translate_version "$version" "$source_fvi" "$target_fvi" forward) || { printf 'observed version has an unmapped lane\n' >&2; exit 1; }
  round_trip=$(translate_version "$translated" "$target_fvi" "$source_fvi" inverse) || exit 1
  [[ "$round_trip" == "$version" ]] || { printf 'round-trip validation failed\n' >&2; exit 1; }
done <"$work_dir/observed-versions"

# Prove the mapping also commutes with representative future epochs, not only
# with the bounded versions returned by DescribeNamespace.
while IFS=$'\t' read -r source_lane target_lane; do
  ((source_fvi <= (9223372036854775807 - source_lane) / 2)) || { printf 'source FVI is too large for future-version validation\n' >&2; exit 1; }
	((target_fvi <= (9223372036854775807 - target_lane) / 2)) || { printf 'target FVI is too large for future-version validation\n' >&2; exit 1; }
  future_source=$((2 * source_fvi + source_lane))
  future_target=$(translate_version "$future_source" "$source_fvi" "$target_fvi" forward) || exit 1
  expected_target=$((2 * target_fvi + target_lane))
  [[ "$future_target" == "$expected_target" ]] || { printf 'future failover validation failed\n' >&2; exit 1; }
  future_round_trip=$(translate_version "$future_target" "$target_fvi" "$source_fvi" inverse) || exit 1
  [[ "$future_round_trip" == "$future_source" ]] || { printf 'future failback validation failed\n' >&2; exit 1; }
done <"$work_dir/mapping.tsv"

# Use the same canonical representation as failoverversion.Mapping.Descriptor.
# The fingerprint lets operators and tests compare generated evidence with all
# running owner replicas without exposing the complete lane mapping in debug
# endpoints or metric labels.
printf 'v1|%s|%s' "$source_fvi" "$target_fvi" >"$work_dir/mapping.canonical"
while IFS=$'\t' read -r source_lane target_lane; do
  printf '|%s:%s' "$source_lane" "$target_lane" >>"$work_dir/mapping.canonical"
done <"$work_dir/mapping.tsv"
if command -v sha256sum >/dev/null 2>&1; then
  mapping_fingerprint=$(sha256sum "$work_dir/mapping.canonical" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  mapping_fingerprint=$(shasum -a 256 "$work_dir/mapping.canonical" | awk '{print $1}')
else
  printf 'sha256sum or shasum is required to fingerprint the mapping\n' >&2
  exit 1
fi

printf 'source: %s uuid=%s fvi=%s ifv=%s\n' "$source_name" "$source_uuid" "$source_fvi" "$source_ifv" >&2
printf 'target: %s uuid=%s fvi=%s ifv=%s\n' "$target_name" "$target_uuid" "$target_fvi" "$target_ifv" >&2
printf 'namespace: %s id=%s failoverVersion=%s\n' "$namespace" "$namespace_id" "$namespace_version" >&2
printf 'warning: DescribeNamespace failover history is bounded; supply omitted lanes with --historical-source-ifv\n' >&2
printf 'validated observed-version round trips, future failover/failback, and order-preserving lane allocation\n' >&2
printf 'mapping-fingerprint: sha256:%s\n' "$mapping_fingerprint" >&2

printf 'failoverVersionTranslation:\n'
printf '  fvi:\n'
printf '    local: %s\n' "$source_fvi"
printf '    remote: %s\n' "$target_fvi"
printf '  ifv:\n'
printf '    mappings:\n'
while IFS=$'\t' read -r local_lane remote_lane; do
  printf '      - local: %s\n' "$local_lane"
  printf '        remote: %s\n' "$remote_lane"
done <"$work_dir/mapping.tsv"
