---
name: check-namespace-fields
description: Comprehensively scan ALL replication tasks and ALL forwarded admin/workflow APIs for namespace-name fields the s2s-proxy translation layer must handle. Use after a go.temporal.io/server upgrade, or whenever asked whether replication tasks, history events, or forwarded RPC request/response messages carry namespace fields that need translation. Flags any namespace-name field not covered by the translation maps.
tools: Read, Glob, Grep, Bash, Write
---

# Check namespace fields in replication tasks and forwarded APIs

The s2s-proxy translation layer ([interceptor/reflection.go](../../../interceptor/reflection.go)) rewrites
namespace names inside replicated payloads and forwarded RPCs so a namespace can be renamed across the proxy
hop. This skill does a **full audit** — it recursively walks every replication task and every forwarded
admin/workflow RPC request/response (including nested messages) and flags any namespace-**name** field the
translator wouldn't rewrite.

## How translation decides what to rewrite

`visitNamespace` in [interceptor/reflection.go](../../../interceptor/reflection.go) reflectively walks a message
and rewrites:

1. **By type** — a `*namespace.NamespaceInfo` value → its `.Name` field.
2. **By field name** — a `string` field whose Go name is in `namespaceFieldNames`: `Namespace`,
   `WorkflowNamespace`, `ParentWorkflowNamespace`. In proto terms: `namespace`, `workflow_namespace`,
   `parent_workflow_namespace`.
3. **Data blobs** — a field named in `dataBlobFieldNames` (`Events`, `NewRunEvents`, `EventBatch`,
   `EventBatches`, `EventsBatches`, `HistoryBatches`) → serialized history is deserialized and recursed.
4. **History events** — `namespaceTranslationSkippableHistoryEvents` lists event types with NO namespace field
   (skipped for efficiency); a populated `HistoryEvent.Links[].WorkflowEvent.Namespace` always forces translation.

**IDs are never translated.** Any field named `*_id` / `*_ids` (e.g. `namespace_id`, `namespace_ids`) holds an
ID, not a name, and is intentionally left alone.

So a namespace field needs attention only if it is a `string` whose proto name contains `namespace`, is **not**
an `_id`/`_ids`, and is **not** one of the three covered names — i.e. a namespace **name** under an unexpected
field name. The scanner below reports exactly those as `UNCOVERED`.

## Procedure

### 1. Build the forwarded-method allow-lists from the proxy source
The scan covers exactly the RPCs the proxy actually forwards (all methods defined on the proxy servers):
```bash
grep -oE "func \(s \*workflowServiceProxyServer\) [A-Za-z0-9]+" proxy/workflowservice.go \
  | awk '{print $NF}' | sort -u > /tmp/wf_forwarded.txt
grep -oE "func \(s \*adminServiceProxyServer\) [A-Za-z0-9]+" proxy/adminservice.go \
  | awk '{print $NF}' | sort -u > /tmp/ad_forwarded.txt
```
(If either file is absent when the scanner runs, that service's ALL methods are scanned instead.)

### 2. Write and run the scanner
Create `tmpnsscan/main.go` inside the repo module (so it resolves the temporal deps), run it, then remove it.
It walks — recursively, following nested messages, guarding against cycles:
- **All replication tasks**: the `ReplicationTask` oneof (every task type + their nested trees).
- **All forwarded workflow + admin RPCs**: each method's request and response message trees.

```go
package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	// Side-effect imports register the proto descriptors we scan.
	_ "go.temporal.io/api/workflowservice/v1"
	_ "go.temporal.io/server/api/adminservice/v1"
	_ "go.temporal.io/server/api/replication/v1"
)

// Proto field names the translator rewrites (Go: Namespace/WorkflowNamespace/ParentWorkflowNamespace).
var covered = map[string]bool{
	"namespace":                 true,
	"workflow_namespace":        true,
	"parent_workflow_namespace": true,
}

type counts struct {
	covered, id, uncovered int
	uncoveredPaths         []string
}

func walk(md protoreflect.MessageDescriptor, path string, visited map[protoreflect.FullName]bool, c *counts) {
	if visited[md.FullName()] {
		return
	}
	visited[md.FullName()] = true
	defer delete(visited, md.FullName())
	fs := md.Fields()
	for i := 0; i < fs.Len(); i++ {
		fd := fs.Get(i)
		name := string(fd.Name())
		if fd.Kind() == protoreflect.StringKind && strings.Contains(name, "namespace") {
			switch {
			case strings.HasSuffix(name, "_id") || strings.HasSuffix(name, "_ids"):
				c.id++ // ID field, intentionally not translated
			case covered[name]:
				c.covered++
			default:
				c.uncovered++
				c.uncoveredPaths = append(c.uncoveredPaths, path+name)
			}
		}
		if fd.Kind() == protoreflect.MessageKind || fd.Kind() == protoreflect.GroupKind {
			walk(fd.Message(), path+name+".", visited, c)
		}
	}
}

func scanMessage(fullName, label string, c *counts) {
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(fullName))
	if err != nil {
		fmt.Printf("!! %s: %v\n", fullName, err)
		return
	}
	walk(d.(protoreflect.MessageDescriptor), label+".", map[protoreflect.FullName]bool{}, c)
}

func scanService(fullName string, methods map[string]bool, c *counts) {
	d, err := protoregistry.GlobalFiles.FindDescriptorByName(protoreflect.FullName(fullName))
	if err != nil {
		fmt.Printf("!! %s: %v\n", fullName, err)
		return
	}
	ms := d.(protoreflect.ServiceDescriptor).Methods()
	for i := 0; i < ms.Len(); i++ {
		m := ms.Get(i)
		if methods != nil && !methods[string(m.Name())] {
			continue
		}
		walk(m.Input(), string(m.Name())+"Request.", map[protoreflect.FullName]bool{}, c)
		walk(m.Output(), string(m.Name())+"Response.", map[protoreflect.FullName]bool{}, c)
	}
}

// readSet returns the method allow-list, or nil (scan all methods) if the file is absent.
func readSet(path string) map[string]bool {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	m := map[string]bool{}
	s := bufio.NewScanner(f)
	for s.Scan() {
		if l := strings.TrimSpace(s.Text()); l != "" {
			m[l] = true
		}
	}
	return m
}

func report(label string, c counts) {
	fmt.Printf("== %s: covered=%d id=%d uncovered=%d\n", label, c.covered, c.id, c.uncovered)
	sort.Strings(c.uncoveredPaths)
	for _, p := range c.uncoveredPaths {
		fmt.Printf("   UNCOVERED: %s\n", p)
	}
}

func main() {
	var rep, wf, ad counts
	scanMessage("temporal.server.api.replication.v1.ReplicationTask", "ReplicationTask", &rep)
	scanService("temporal.api.workflowservice.v1.WorkflowService", readSet("/tmp/wf_forwarded.txt"), &wf)
	scanService("temporal.server.api.adminservice.v1.AdminService", readSet("/tmp/ad_forwarded.txt"), &ad)

	report("replication tasks (all)", rep)
	report("workflow forwarded APIs", wf)
	report("admin forwarded APIs", ad)

	total := rep.uncovered + wf.uncovered + ad.uncovered
	fmt.Printf("\n=== TOTAL UNCOVERED namespace-name fields: %d ===\n", total)
	if total > 0 {
		os.Exit(1)
	}
}
```

Run and clean up:
```bash
go run ./tmpnsscan   # exits non-zero and lists paths if any UNCOVERED field is found
rm -rf tmpnsscan
```

### 3. Interpret and patch
- **`uncovered=0`** everywhere → the translation maps fully cover the current surface; no change needed.
- **Any `UNCOVERED:` path** → a namespace-**name** field the translator would miss. Confirm it truly holds a
  name (not an ID the heuristic mis-slotted), then in [interceptor/reflection.go](../../../interceptor/reflection.go):
  - new namespace-name field → add its Go field name to `namespaceFieldNames`.
  - new history-event data blob reached via an unlisted field → add the field name to `dataBlobFieldNames`.
  - new history event type with NO namespace field → add to `namespaceTranslationSkippableHistoryEvents`
    (with a test in [interceptor/reflection_test.go](../../../interceptor/reflection_test.go) asserting
    `isSkippableForNamespaceTranslation`).
- Then `go test -tags test_dep ./interceptor/...` and `make lint`.

## Notes / gotchas
- Translation matches **field names**, not proto field numbers — renamed Go fields matter.
- `namespace_id` **and** `namespace_ids` (plural) are IDs — never translate them; the scanner classifies both as `id`.
- Search-attribute translation is a parallel mechanism (`searchAttributeFieldNames`, `visitSearchAttributes`);
  extend the scanner's `covered`/heuristic if you want to audit `search_attributes` too.
- Touching any file in `interceptor/` re-lints the whole package and can surface pre-existing `staticcheck`
  issues (e.g. `SA1019`, `SA9003`) — fix or `//nolint` with a reason so `make lint` stays green.
- A namespace-name field reached only inside a serialized `DataBlob` is translated only if the blob's container
  field is in `dataBlobFieldNames`; the scanner sees the structured descriptor, so cross-check blob paths there.
