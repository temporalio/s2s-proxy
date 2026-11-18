# Target FVI namespace handover prototype

This opt-in prototype rewrites namespace metadata at the proxy boundary. It does not change Temporal server code or existing workflow history. It is intended for local validation, not production rollout.

The proxy handles both `GetNamespaceReplicationMessages` and `GetNamespace` so namespace polling and eager refresh expose the same cutover version. The current namespace version and relevant failover-history entries are mapped together. Matching uses the namespace and active-cluster names returned by the upstream server, before response translation.

## Configuration

For a single TCP proxy whose local cluster is S (IFV 1, FVI 7) and remote cluster is C (IFV 2, FVI 10):

```yaml
clusterConnections:
  - name: target-fvi
    local:
      connectionType: tcp
      tcpClient:
        address: 127.0.0.1:7233
      tcpServer:
        address: 127.0.0.1:38233
    remote:
      connectionType: tcp
      tcpClient:
        address: 127.0.0.1:8233
      tcpServer:
        address: 127.0.0.1:37233
    failoverVersionIncrementTranslation:
      local: 10
      remote: 7
    namespaceFailoverVersionRewrite:
      inbound:
        - namespace: fvi-validation
          targetCluster: cluster-fvi-c
          sourceInitialFailoverVersion: 1
          sourceIncrement: 7
          targetInitialFailoverVersion: 2
          targetIncrement: 10
          minVersion: 9
      outbound:
        - namespace: fvi-validation
          targetCluster: cluster-fvi-s
          sourceInitialFailoverVersion: 2
          sourceIncrement: 10
          targetInitialFailoverVersion: 1
          targetIncrement: 7
          minVersion: 21
```

The existing FVI translation fields change `DescribeCluster` responses so the clusters can connect despite unequal increments. The new fields change namespace metadata only. Other namespaces and active owners are unaffected. Use a migration-specific namespace/epoch scope; the local fixture uses `minVersion: 1` to seed its initial epochs automatically.

For a matching source-generated target epoch, choose the next target-owned version above the incoming version, skipping a candidate that identifies the source as self. Native target epochs are left unchanged, making reads and replays idempotent. Source-authored historical epochs are preserved.

## Local validation

Build with the Go toolchain declared by go.mod:

```sh
go build -o bins/s2s-proxy ./cmd/proxy
go build -o bins/fvi-probe ./cmd/tools/fvi-probe
./scripts/generate-certs.sh
go test -tags test_dep ./... -count=1 -timeout=5m
go tool -modfile=develop/tools.mod golangci-lint run --build-tags=test_dep
```

The live probe expects S on port 7233, C on 8233, and the single proxy on 38233/37233. It uses isolated namespaces and the existing server worker's `namespace-handover` workflow. Add rewrite rules for each positive-test namespace before starting the proxy:

```sh
FVI_NAMESPACE=fvi-validation ./bins/fvi-probe
FVI_NAMESPACE=fvi-legacy ./bins/fvi-probe
FVI_NAMESPACE=fvi-control FVI_CONTROL=1 ./bins/fvi-probe
```

Use a fresh namespace for each run. The positive namespaces must have rules in both directions. The negative-control namespace must have no rewrite rules. For `fvi-legacy`, set `history.enableTransitionHistory: false` with a namespace constraint on both servers; retain `true` for the normal test. Existing namespaces already advanced beyond the fixture's initial version 8 are not reset by the probe.

The probe seeds version 8 via namespace updates, runs a signal-driven workflow with activities and timers, performs S to C handover (9 becomes 12), then C to S handover (21 becomes 29), and completes the workflow. Version 22 is skipped on the reverse path because `22 % 10 == 2` would identify C as self. Both complete histories must match event for event, including the original version-8 events.

With transition history enabled, namespace versions converge through eager refresh (9/12 becomes 12/12, then 29/21 becomes 29/29). With it disabled, the tested event-based path processes newer workflow versions while namespace metadata remains temporarily unequal. The negative control checks that Cloud rejects workflow creation with `unknown-cluster-9` when the rewrite is absent, after allowing its namespace cache to catch up.

## Omes load results

A local `throughput_stress` run used the same unequal-FVI clusters, clients pinned to S, and one Go worker on each cluster. Over 180 seconds of offered load (maximum 6 concurrent iterations and 1 new iteration/second), four handovers completed: S to C to S to C to S.

- 115 successful iterations produced 920 executions, including 690 child executions and 115 continue-as-new successors.
- All 74,958 history events and all execution statuses matched across both clusters; 21 histories crossed a version boundary.
- No unknown-cluster messages appeared in the captured server/proxy logs. Both history DLQ listings were empty after the run.
- Transient replication resync and handover retries recovered. Canceled forwarded polls also occurred during namespace-cache disagreement, which can occur in ordinary handover; no equal-FVI control was run to attribute their frequency to this prototype.

This run used OSS server revision `f93f567d8395042b132ff69685053e0282e15416` (reports 1.32.0) and Omes revision `2e00fcc313f85861172323fd0da929bb7c04f64a`. Nexus and standalone activities were disabled. The scenario covered activities, local activities, timers, signals, updates, queries, describe calls, children and continue-as-new. These short local results do not establish production readiness.

## Limits

This is a stateless prototype scoped to sequential planned handovers. It is not a general mapping of every historical author or a coordination protocol for concurrent failovers. Production work still needs historical collision analysis for the actual IFVs, replication-clock validation, all relevant namespace/bootstrap paths, multi-proxy directionality, and durable migration-attempt/replay semantics. The local C cluster is an OSS stand-in, not the Cloud control plane or MCN implementation.
