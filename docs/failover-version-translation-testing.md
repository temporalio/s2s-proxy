# Failover-version translation testing and observability

This document separates implemented behavior from proof. A code path is not
considered covered merely because a large migration scenario traverses it.

Status:

- **covered**: a focused automated test passes;
- **partial**: only some inputs or an indirect assertion exist;
- **manual**: a real-cluster test exists but is not a CI gate;
- **missing**: the required proof has not been implemented.

## Reusable testability interfaces

| Interface | Purpose |
|---|---|
| Transactional `SemanticTranslator` | Translates a clone and returns a `TranslationReport`; no failed request, response, stream message, status, or blob list can mutate its input. |
| `Mapping.Descriptor` | Produces the versioned canonical fingerprint and safe mapping shape used by generation, debug state, and the harness. |
| `ValidateSuperset` | Prevents FVI changes, pair removal, or pair renumbering after translated persistence. |
| Semantic field registry | Makes every explicit protobuf path and exclusion inspectable; descriptor tests fail when paths disappear or new failover-version candidates arrive. |
| `TranslationObserver` | Feeds bounded metrics, structured logs, a 100-entry sanitized history, lifetime direction/operation counters, and isolated test recorders without exposing payload values. |
| `HistoryEventCodec` option | Allows deterministic decode/encode failure injection without substituting production serialization globally. |
| `/debug/connections` translation state | Shows owner/pass-through role, fingerprint, shape, directions, recent outcomes, lifetime operation counts, namespace queue cursors/task counts, active replication watermarks, and Go memory/goroutine state. |
| Harness `LocalFrontendClient` | Disables XDC forwarding on every call so target checks cannot be answered by the active cluster. |
| Harness `TopologySnapshot` and `Checkpoint` | Capture proxy wiring, mapping identity, local namespace membership/state, normalized histories, version domains, pending state, queue cursors, process/container health, faults, and bounded logs as JSON artifacts. |
| `WorkloadManifest` validators | Prove that required lanes, epochs, executions, pending state, and event counts exist before migration and converge afterward. |
| `StopProxies` / `RestartProxySide` | Exercise full, owner-only, and peer-only restarts. Side replacement re-registers a generation-specific container address so Temporal evicts ClientConns that cached the old pod IP. |
| Per-peer `AdminClientForPeer` route probes | Prove every listener in an N-cluster proxy group reaches its intended peer instead of repeatedly testing only the first route. |
| Namespace-consumer quiescence | Disables the old remote connection and waits for metadata propagation before a same-name cluster/database is recreated, preventing old queue cursors from acknowledging past a fresh queue. |
| Scheduler run/ready gates | Distinguish acceptance of a force-continue-as-new signal from completion of the new scheduler run and its ability to process updates. |
| Runtime and container observations | Preserve heap, released pages, goroutine/stack counts, exit code, dead state, and OOM evidence even when the debug endpoint disappears. Proxy tests run with a 1 GiB soft Go memory limit. |
| Reversible `FaultHandle` | Disconnects a proxy side from the Docker network, restores all pod/shared aliases, records timing, and always registers cleanup. |

Set `COMPATIBILITY_TEST_ARTIFACT_DIR` to retain checkpoint JSON outside the
test temporary directory. Failed topologies automatically capture a final
proxy/debug snapshot before teardown.

## Coverage matrix

### 1. Configuration and scalar mathematics

| Line item | Status | Existing proof | Testability or next test |
|---|---|---|---|
| Invalid FVI/IFV, empty/duplicate/out-of-domain lanes, order inversion, legacy conflict | partial | Config validation table | Add missing-sub-block, negative lane, multi-connection field path, round-trip, and validation non-mutation cases. |
| Zero, endpoint/canonical lanes, high epochs, exact forward/inverse | partial | Unit/property/fuzz tests | Expand the independent scalar oracle around every epoch boundary and both lane orderings. |
| Negative, unknown, unexpected FVI, int32/int64 overflow | partial | Focused error tests | Exercise every scalar method in both directions. |
| Immutable concurrent use | covered | Race-enabled shared mapping test | Keep directional views immutable. |
| Mapping identity and safe update | covered | Canonical descriptor and superset unit tests in both repositories | Compare the generator, all owner replicas, and artifacts during the manual scenario. |
| Error wrapping and redaction | partial | Typed/redacted overflow and field tests | Table-test every error code and cause location through the recording observer. |

### 2. Protobuf, status, and blob translation

| Line item | Status | Existing proof | Testability or next test |
|---|---|---|---|
| Discovery, namespace, mutable state, one history blob | partial | Focused reversible fixtures | Add one fixture for every registry path and assert the exact report category. |
| Reset fork and parent-initiated versions | covered | Descriptor registry resolves their actual linked API paths | Add value-level fixtures in addition to descriptor existence. |
| Pending activity/timer/child/cancel/signal and task info | missing | None beyond activity fields | Add valid persistence fixture builders and inverse equality tests. |
| Versioned transitions, HSM, CHASM, tombstones, backfill | missing | Recursive implementation only | Add nested map/list/oneof fixtures tied to registry ownership. |
| Clock IDs and unrelated-version exclusions | partial | HLC/config exclusion fixture | Cover every explicit exclusion and target-cluster ID request. |
| Multiple history encodings and batches | partial | Proto3 single-blob test | Use the codec option for JSON, empty, multiple, new-run, raw-history, and repair cases. |
| Later event/blob decode, encode, or lane failure | partial | Transactional later-blob test | Add fail-at-N codec tests and encoding-preservation assertions. |
| Opaque replication data | partial | Non-empty rejection | Add empty acceptance and transport non-forwarding. |
| Multiple/malformed status details | partial | Successful and atomic later-detail tests | Add unknown detail pass-through and classified malformed `Any`. |
| Schema drift | covered | Linked-descriptor registry audit | Expand candidate policy if Temporal introduces semantic fields with novel names. |

### 3. Interceptors, metadata, and wiring

| Line item | Status | Existing proof | Testability or next test |
|---|---|---|---|
| Unary request/response/error order and atomic commit | covered/partial | Clone/commit interceptor and order tests | Add nil/non-protobuf responses and handler-error precedence cases. |
| Stream receive/send/EOF/failure | partial | EOF and failed-send tests | Add successful commit, concurrent streams, and disconnect-at-boundary tests. |
| Cluster-ID metadata validation and cloning | covered/partial | Malformed table and clone assertion | Add non-decimal, unmapped positive, inverse, and post-routing normalization. |
| Owner-only/pass-through and fingerprint | covered in harness units; manual runtime | Config and runtime debug validator | Retain both mux-role unit cases and validate every live replica. |
| Double translation | missing | Owner-only construction is indirect | Use an overlapping non-idempotent mapping through TCP, mux, routing, and intra-proxy paths. |
| Raw versus translated discovery | missing | Raw bypass interceptor only | Query both views through a running proxy and compare FVI/IFV. |
| Metrics, logs, and debug redaction | partial | Bounded metric/observer assertions and debug test | Add full error-code metric table and captured-log secret fixtures. |

### 4. Generator and preflight

| Line item | Status | Existing proof | Testability or next test |
|---|---|---|---|
| Standard allocation/current CLI/fingerprint | covered | Fake CLI and cross-implementation fingerprint test | Retain as a normal unit test. |
| CLI/JSON/argument failures and casing/int64 variants | missing | Minimal happy-path fixtures | Replace embedded fake scripts with a scenario builder and table. |
| Source namespace/global/active/sole-replica checks | implementation present; partial tests | Manual scenario plus happy path | Add one black-box rejection per precondition. |
| Existing target namespace | implementation present; missing focused test | Manual target absence | Add exact target-present and unrelated CLI failure fixtures. |
| Recreated UUID and bounded history | partial | Manual scenario and warning behavior | Assert UUID evidence, explicit historical lanes, and warning codes. |
| Occupied/virtual/impossible allocation | missing | Allocator used by happy path | Compare small domains with an independent brute-force Go oracle. |
| Arithmetic/shell limits and cleanup | missing | Exact string parsing implementation | Add signed-int64 and IEEE-754 boundary fixtures plus temp cleanup checks. |

### 5. Fresh-target bootstrap and integrity

| Line item | Status | Existing proof | Testability or next test |
|---|---|---|---|
| Recreated empty target with unequal FVI | manual, verified | Ordered pinned-1.31.1 scenario reaches bootstrap, translated failover, native B writes, translated failback, and post-restart convergence | Retain database/container identity, queue-cursor, and generator-report artifacts for each release combination. |
| Local namespace and workflow presence | manual, verified | `LocalFrontendClient` and successful fresh-target/failback checkpoints | Add more execution shapes. |
| Exact execution/event/payload preservation | manual, newly asserted for selected workflows | Normalized history digest convergence | Extend the manifest to dormant and closed executions. |
| State-based/sync-state/last-history path evidence | partial | Bootstrap is forced and proxy stream watermarks/acks are captured; task type is not observed | Add independent replication-task type observations. |
| Raw history, mutable state, transitions | partial | Independent mutable-state normalization captures semantic versions, lane shape, transition count, checksum, and entity counts | Add raw-history branch/task collectors and compare mutable-state digests in every migration phase. |
| Pending activity/timer/child/cancel/signal | mostly missing | Manifest supports activity/child counts | Add workloads and collectors for timers, cancellation, and signals. |
| Continue-as-new/reset/branch/schedule/HSM | partial/manual | Schedule and continue-as-new progress | Add branch digests and schedule/HSM state snapshots. |
| Visibility versus persistence | partial | Readiness uses local history, not visibility | Record visibility separately with bounded eventual lag. |

### 6. Disconnect, reconnect, failover, and recovery

| Line item | Status | Existing proof | Testability or next test |
|---|---|---|---|
| Mux receiver stop/restart | covered | Focused bidirectional integration test closes and recreates the receiver on the same address; server lifecycle regression ensures a failed listener is recycled rather than spin-retried | Add repeated/flapping and in-flight stream variants. |
| Full/owner/peer proxy restart | manual, verified | `RestartProxies`, `RestartProxySide`, per-peer unary routing, generation-specific remote re-registration, bidirectional stream readiness, fingerprint checkpoints | Vary restart timing and pod count. |
| Symmetric proxy-side partition and backlog | manual, verified | Cleanup-registered fault, writes during partition, digest convergence, proxy stream watermark snapshots | Assert exact expected watermark movement. |
| Same-name fresh database with stale namespace cursor | manual regression covered | Old connection is disabled before replacement; metadata deletion is observed before re-registration; debug artifacts expose request/response cursors and task counts | Add a negative test that deliberately omits quiescence and asserts the stale-cursor signature without corrupting a shared environment. |
| History streams healthy while namespace polling is stale | covered by observations; manual regression | Stream readiness and namespace operation/cursor progress are reported independently | Add a reusable active namespace canary that does not alter customer metadata. |
| Split active-cluster view with live long pollers | manual regression covered | Worker quiescence prevents A↔B redirection loops while namespace failover tasks converge; lifetime operation counts and goroutine/stack state expose amplification | Add a bounded negative test and proxy hop/loop guard. |
| Scheduler failover/continue-as-new readiness | manual | Local run-ID change and retried identity update separate namespace convergence from scheduler workflow convergence | Add schedule state/history digests before and after failback. |
| Translation clone retention/OOM | covered/partial | Race test translates repeated 4 MiB history payloads, forces GC, and bounds retained heap; runtime/container snapshots preserve OOM evidence | Add long-running RSS/heap profiles and concurrent stream load. |
| Directional loss, latency, resets, flapping | missing | Fault abstraction supports extension | Add a fault-proxy implementation rather than privileged container mutation. |
| In-flight unary/stream cancellation | missing | Transactional boundary API exists | Inject transport closure at receive/send commit points. |
| Ack/watermark non-advancement on translation error | partial collector, missing fault test | Translation errors plus proxy stream ack/watermark state are observable | Add source/target persistence watermarks and an incomplete-mapping recovery test. |
| DLQ, retention, and expiration | missing | No collector | Add admin replication/DLQ observation and a destructive manual recovery drill. |
| B writes, failback, and A replay | manual, verified | B-native workflow completes; A/B local normalized history converges after translated failback and full mesh restart; scheduler continues as new on A | Add mutable-state replay evidence for the B-created execution. |
| Repeated translated cycles and soak variation | missing/partial | Soak repeats the same ordered test | Parameterize restart side, partition timing, backlog size, and failover direction. |

## Required commands

Core deterministic checks:

```shell
go test -race -tags test_dep -count=1 ./failoverversion ./config ./interceptor ./proxy/...
go test -count=1 ./scripts/test
make lint
```

Harness units and manual migration:

```shell
make test-compatibility-unit
COMPATIBILITY_TEST_ARTIFACT_DIR=/absolute/artifacts \
make test-failover-version-translation \
  COMPATIBILITY_TEST_PROXY_SOURCE=/absolute/path/to/s2s-proxy
```

The real-cluster test remains manual. It was last verified against Temporal
`1.31.1` on 2026-11-18; this is not a substitute for retaining the checkpoint
bundle and successful command output for each proxy/Temporal version
combination.
