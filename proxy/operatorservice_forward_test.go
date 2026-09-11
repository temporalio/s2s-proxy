package proxy

import (
	"context"
	"reflect"
	"testing"

	"go.temporal.io/api/operatorservice/v1"
)

// TestAllOperatorMethodsForwarded guards against silently dropping new RPCs on a
// go.temporal.io/api upgrade. The embedded UnimplementedOperatorServiceServer
// makes such gaps compile cleanly, so the compiler cannot catch them; its
// default implementation returns codes.Unimplemented.
//
// The proxy is built as a zero value, so operatorServiceClient/logger are nil. A
// real forwarding implementation panics on the nil client/logger before
// returning; the embedded default returns codes.Unimplemented without touching
// them. Any outcome other than a clean codes.Unimplemented therefore means the
// method is explicitly implemented. See returnsUnimplemented in
// adminservice_forward_test.go.
func TestAllOperatorMethodsForwarded(t *testing.T) {
	iface := reflect.TypeFor[operatorservice.OperatorServiceServer]()
	srv := reflect.ValueOf(&operatorServiceProxyServer{})
	ctxType := reflect.TypeFor[context.Context]()

	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)

		// Skip the unexported mustEmbedUnimplementedOperatorServiceServer() marker
		// method, which has no params/returns.
		if m.PkgPath != "" {
			continue
		}

		// Build call args from the method signature (no receiver on interface
		// methods): a real context where the parameter is a context.Context
		// (every unary RPC), and a zero value elsewhere.
		mt := m.Type
		in := make([]reflect.Value, mt.NumIn())
		for j := 0; j < mt.NumIn(); j++ {
			if mt.In(j) == ctxType {
				in[j] = reflect.ValueOf(context.Background())
			} else {
				in[j] = reflect.Zero(mt.In(j))
			}
		}

		if returnsUnimplemented(srv.MethodByName(m.Name), in) {
			t.Errorf("OperatorService.%s is not explicitly forwarded (falls through to "+
				"UnimplementedOperatorServiceServer). Add a pass-through in operatorservice.go.", m.Name)
		}
	}
}
