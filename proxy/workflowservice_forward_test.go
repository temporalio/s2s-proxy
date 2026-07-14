package proxy

import (
	"context"
	"reflect"
	"testing"

	"go.temporal.io/api/workflowservice/v1"
)

// TestGlobalAPIsForwarded guards the forwarding contract for the workflow proxy:
// every API listed in globalAPIResponses must be explicitly forwarded by
// workflowServiceProxyServer rather than falling through to the embedded
// UnimplementedWorkflowServiceServer default (which returns codes.Unimplemented).
func TestGlobalAPIsForwarded(t *testing.T) {
	iface := reflect.TypeFor[workflowservice.WorkflowServiceServer]()
	srv := reflect.ValueOf(&workflowServiceProxyServer{})
	ctxType := reflect.TypeFor[context.Context]()

	// Index the interface methods by name so we can look up each global API.
	methods := make(map[string]reflect.Method, iface.NumMethod())
	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		methods[m.Name] = m
	}

	for name := range globalAPIResponses {
		m, ok := methods[name]
		if !ok {
			// A global API that no longer exists on the interface (renamed or
			// removed upstream) is stale and should be pruned from the map.
			t.Errorf("globalAPIResponses[%q] is not a WorkflowService RPC (stale entry?)", name)
			continue
		}

		mt := m.Type
		in := make([]reflect.Value, mt.NumIn())
		for j := 0; j < mt.NumIn(); j++ {
			if mt.In(j) == ctxType {
				in[j] = reflect.ValueOf(context.Background())
			} else {
				in[j] = reflect.Zero(mt.In(j))
			}
		}

		if returnsUnimplemented(srv.MethodByName(name), in) {
			t.Errorf("WorkflowService.%s is in globalAPIResponses but is not explicitly "+
				"forwarded (falls through to UnimplementedWorkflowServiceServer). "+
				"Add a pass-through in workflowservice.go.", name)
		}
	}
}
