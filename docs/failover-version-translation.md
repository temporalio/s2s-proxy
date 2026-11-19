# Failover-version translation

Failover-version translation lets one proxy connection safely replicate between
Temporal clusters whose failover-version increments differ. It translates the
increment, initial-version lanes, workflow and namespace failover versions,
version histories, mutable state, replication payloads, status details, clocks,
and replication-stream cluster IDs in both directions.

Only the proxy deployment that owns the connection is configured. Its peer
must omit the block so versions are translated exactly once:

```yaml
clusterConnections:
  - name: migration-a-to-b
    # local and remote transport configuration omitted
    failoverVersionTranslation:
      fvi:
        local: 105
        remote: 100
      ifv:
        mappings:
          - local: 1
            remote: 1
          - local: 2
            remote: 2
```

`local` is the native domain of the Temporal cluster behind the owner proxy;
`remote` is the domain presented across the peer boundary. The mapping must be
a strictly order-preserving partial bijection. Version zero is preserved.
Unknown lanes, overflow, opaque future replication payloads, and malformed
stream identities fail the RPC instead of crossing untranslated.

The legacy `failoverVersionIncrementTranslation` setting remains available for
DescribeCluster compatibility only. It cannot be combined with semantic
`failoverVersionTranslation`.

## Generate a mapping

Use native Temporal CLI profiles for both clusters:

```shell
scripts/generate-failover-version-translation.sh \
  --source-profile cluster-a \
  --target-profile cluster-b \
  --namespace migration-namespace
```

The script writes only the YAML fragment to stdout and its discovery report to
stderr. The report includes a canonical `mapping-fingerprint` that must match
the owner proxy's debug state on every replica. It requires `temporal`, `jq`,
Bash, `sort`, and either `sha256sum` or `shasum`. Supply a removed
historical lane that no longer appears in cluster registration or bounded
namespace history with repeatable `--historical-source-ifv` arguments.

Before generation, the source namespace must be global, active on the source,
and have only the source as a current replica. The target namespace must not
exist. Do not change namespace topology while discovery runs.

If the target reuses the name of a destroyed cluster, disable the source's old
remote-cluster connection and wait until every Temporal worker service has
observed that metadata change before starting the fresh target. A namespace
replication consumer retains source-queue message IDs in memory. Allowing it to
reach a fresh queue under the old name can acknowledge an old high message ID
into that queue and permanently skip its new tasks. Retain the disabled
registration through generation so the script can correlate the old and new
cluster UUIDs; then remove it, wait for metadata eviction, and register the
fresh target through the translated proxy boundary.

## Installation and rollback

1. Save the generated YAML and report with the change record.
2. Deploy the owner proxy with the block and the peer proxy without it.
3. Compare raw discovery using `s2s-request-translation=false` with normal
   translated DescribeCluster/ListClusters calls.
4. Re-register the clusters through the proxy, add the target replica, wait for
   replication to catch up, and only then fail over.
5. Quiesce external pollers during each namespace ownership transition unless
   the deployment has a loop guard: opposite active-cluster views can redirect
   the same long poll A-to-B and B-to-A until the namespace task converges.

The mapping is immutable once the target persists translated data. To add a
lane, stop replication, regenerate a strict superset that preserves every old
pair, deploy it, and resume. Never renumber an existing pair.

Before the target becomes a replica, rollback can remove the block. After the
target persists translated state, disabling translation is unsafe. Keep or
fail back to the source through the translator, stop replication, remove the
target replica, discard/rebuild its data, correct the mapping, and bootstrap it
again. An unknown-lane error requires an additive mapping update, never
pass-through behavior.

## Observation and diagnosis

`/debug/connections` includes `failover_version_translations` for every cluster
connection. An owner entry reports the mapping schema, local/remote FVI, lane
count, canonical fingerprint, request/response directions, a bounded set of
sanitized translation observations, lifetime counts per direction/operation,
and namespace-replication request/response cursors and task counts. The same
response reports active stream watermarks and Go heap/stack/goroutine state.
It deliberately does not expose individual lane pairs, namespace names,
payloads, or raw failover versions. A peer without the block reports
`pass_through`.

The dedicated metrics are:

- `failover_version_translation_operations_total{direction,outcome,reason}`;
- `failover_version_translation_fields_total{direction,semantic}`.

All label values are bounded enums. Detailed protobuf field paths remain in
the internal debug response and structured error logs, not metric labels.

Compare these three fingerprints before adding the target replica and again
after any restart:

1. the generator report;
2. every owner proxy replica's debug response;
3. the retained migration artifact.

See [failover-version translation testing](failover-version-translation-testing.md)
for the coverage matrix, harness interfaces, and remaining failure drills.
