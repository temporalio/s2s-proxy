# Namespace cutover failover-version rewrite

## Status

This is an opt-in experiment for controlled handovers between exactly two
Temporal clusters with different failover-version increments (FVIs). It
rewrites namespace metadata only. Workflow history, mutable state, version
histories, transition histories, and workflow replication payloads remain
unchanged.

It is not a replacement for full semantic failover-version translation until
the compatibility tests described below pass for every supported server and
replication mode.

## Example

| Cluster | Initial failover version (IFV) | FVI |
| --- | ---: | ---: |
| self-hosted `S` | 1 | 7 |
| Cloud `C` | 2 | 10 |

The namespace is active on `S` at version 8. During handover, `S` generates 9:

- lane 2 under FVI 7 identifies `C` to `S`;
- lane 9 under FVI 10 does not identify `C` to Cloud.

The proxy changes 9 to 12 before Cloud applies the namespace task. Version 12
is lane 2 under FVI 10, but lane 5—unallocated—in the source domain.

On failback, Cloud generates 21. The first `S`-owned candidate is 22, but that
remains Cloud's lane 2 under FVI 10. The proxy skips it and selects 29, which is
lane 1 under FVI 7 and unallocated lane 9 under FVI 10.

## Configuration

```yaml
clusterConnections:
  - name: self-hosted-to-cloud
    # Transport fields omitted.
    failoverVersionIncrementTranslation:
      # Existing caller-facing overrides are crossed.
      local: 10
      remote: 7
    namespaceFailoverVersionRewrite:
      mode: observe # observe or enforce
      # Canonical local names. namespaceTranslation supplies remote names.
      namespaces:
        - migration-namespace
      local:
        clusterName: self-hosted
        initialFailoverVersion: 1
      remote:
        clusterName: cloud
        initialFailoverVersion: 2
```

The native local FVI is the existing `remote` override (7), and the native
remote FVI is the existing `local` override (10). This avoids configuring the
same arithmetic twice.

`observe` runs the complete matcher and validation path, reports what it would
do, and never changes or rejects a response. `enforce` performs the rewrite
and fails the RPC before mutation if any eligible item is unsafe.

Startup validation requires:

- a non-empty, unique namespace allowlist;
- distinct cluster names, FVIs, and IFVs;
- positive FVIs and IFVs valid in both domains;
- a receiver-owned candidate on an unallocated sender lane in both directions.

## Algorithm and interception

Temporal treats a zero remainder as lane `FVI`:

```text
lane(version, fvi) = ((version - 1) mod fvi) + 1
```

For a response from sender `A` to receiver `B`, the proxy:

1. Passes namespaces not on the allowlist or not containing both configured
   clusters.
2. Rejects an allowlisted topology containing both clusters plus any third
   cluster.
3. Passes unless `B` is active and the input occupies `B.IFV` in `A`'s domain.
4. Selects the first `B`-owned value greater than the input whose lane in
   `A`'s domain is neither registered IFV.

Only three candidates need inspection: there are two allocated sender lanes,
so a third distinct residue is free; if residues repeat sooner, no free lane
exists.

The shared implementation handles:

- `NamespaceTaskAttributes.failover_version` in
  `GetNamespaceReplicationMessages`;
- `GetNamespaceResponse.failover_version` for eager namespace refresh.

When a rewrite is required, the last failover-history entry must equal the
current input version. The proxy updates that entry and the current version,
leaving older history untouched. Replication batches are fully planned before
in-place mutation, so an error cannot expose a partially rewritten batch.

## Operations and observability

Roll out in `observe`, confirm `would_rewrite` for a disposable or planned
cutover, then restart with `enforce` before the real handover. Do not perform a
real handover in observe mode. After a rewritten version is persisted, keep
the feature enabled through failback or until the old cluster is removed.

The proxy emits one bounded counter:

```text
temporal_s2s_proxy_namespace_failover_version_rewrite_total{
  connection,direction,operation,result
}
```

Results are `out_of_scope`, `inactive`, `not_cutover`, `would_rewrite`,
`rewritten`, and `error`. Startup logs report the endpoint arithmetic and
allowlist size. Rewrite logs include canonical namespace, direction, operation,
input, and output; error logs add a bounded reason. Pass-through decisions do
not produce logs, and namespace names are not metric labels.

Before handover, require identical namespace IDs, membership, active owner,
and pre-cutover version, plus caught-up workflow replication. Quiesce writers
that follow redirects until both namespace caches converge. A second handover
must not be used to repair an incomplete first one.

After persistence, rollback is not simply disabling the feature. Stop further
handovers and replication, capture both namespace records and proxy logs, then
rebuild a disposable target or use a reviewed namespace-repair procedure.

## Limits and test gate

The central unresolved risk is an unchanged historical event version mapping
to an unknown lane in the other FVI domain. Compatibility testing must cover
history and state-based replication, eager refresh, resend and DLQ paths,
mutable-state rebuild, restarts, delayed and duplicate namespace tasks,
partitions, repeated failover/failback, and representative workflow features.

Acceptance requires no lost, duplicated, or reinterpreted events; identical
complete histories and application results; no ownership or redirect loop;
and deterministic recovery after reconnect and restart.

The executable two-cluster probe in
[`develop/target-fvi`](../develop/target-fvi/README.md) verifies both handover
directions, writes on each new active cluster, reverse replication, and final
history equality against real Temporal server processes.
