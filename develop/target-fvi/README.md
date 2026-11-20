# Target-FVI live probe

This fixture exercises the namespace cutover rewrite against two real Temporal
servers. It is intentionally separate from the unit tests because it starts
global namespaces, remote-cluster replication, workers, and handover
workflows.

The servers must have these native identities:

| Side | Cluster name | FVI | IFV | Default frontend |
| --- | --- | ---: | ---: | --- |
| Source | `cluster-fvi-s` | 7 | 1 | `127.0.0.1:7233` |
| Cloud | `cluster-fvi-c` | 10 | 2 | `127.0.0.1:8233` |

The probe registers both remote clusters through the proxy, creates a global
namespace, runs a workflow, performs `S -> C` and `C -> S` handovers, writes on
each new active cluster, and compares the complete histories. A successful run
observes `8 -> (9, 12) -> (29, 21) -> 29` in the two native namespace records
and ends with `histories_identical=true`.

The version-8 setup uses an artificial ownership round trip. When the rewrite
is already enforcing, that setup correctly looks like a cutover and is
rewritten. To test an existing version-8 migration baseline, first run the
proxy with the supplied `proxy.yaml` in `observe` mode and seed its allowlisted
namespace:

```sh
FVI_NAMESPACE=fvi-cutover-test FVI_SEED_ONLY=1 go run ./cmd/tools/fvi-probe
```

Change only the proxy mode to `enforce`, restart it, then run the full probe
against the same namespace:

```sh
FVI_NAMESPACE=fvi-cutover-test go run ./cmd/tools/fvi-probe
```

Addresses are overrideable for isolated test processes:

| Variable | Default |
| --- | --- |
| `FVI_SOURCE_ADDRESS` | `127.0.0.1:7233` |
| `FVI_CLOUD_ADDRESS` | `127.0.0.1:8233` |
| `FVI_SOURCE_PROXY_ADDRESS` | `127.0.0.1:38233` |
| `FVI_CLOUD_PROXY_ADDRESS` | `127.0.0.1:37233` |
| `FVI_NAMESPACE` | `fvi-prototype` |

`FVI_CONTROL=1` stops after the first handover and proves the unmodified
version 9 is rejected by Cloud as an unknown owner. Use a fresh namespace and
a pass-through/observe proxy for that negative control.
