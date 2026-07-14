package proxy

import (
	"context"
	"reflect"
	"testing"

	"go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestAllAdminMethodsForwarded guards against silently dropping new RPCs on a
// go.temporal.io/server upgrade. The embedded UnimplementedAdminServiceServer
// makes such gaps compile cleanly, so the compiler cannot catch them; its
// default implementation returns codes.Unimplemented.
//
// The proxy is built as a zero value, so adminClient/loggers are nil. A real
// forwarding implementation panics on the nil adminClient/loggers before
// returning; the embedded default returns codes.Unimplemented without touching
// them. Any outcome other than a clean codes.Unimplemented therefore means the
// method is explicitly implemented.
func TestAllAdminMethodsForwarded(t *testing.T) {
	iface := reflect.TypeFor[adminservice.AdminServiceServer]()
	srv := reflect.ValueOf(&adminServiceProxyServer{})
	ctxType := reflect.TypeFor[context.Context]()

	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)

		// Skip the unexported mustEmbedUnimplementedAdminServiceServer() marker
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
			t.Errorf("AdminService.%s is not explicitly forwarded (falls through to "+
				"UnimplementedAdminServiceServer). Add a pass-through in adminservice.go.", m.Name)
		}
	}
}

// returnsUnimplemented reports whether calling method with args returns a
// codes.Unimplemented status error. A panic (from dereferencing the nil
// adminClient/loggers in a real forwarding method) is recovered and means the
// method is implemented, so it returns false.
func returnsUnimplemented(method reflect.Value, args []reflect.Value) (unimplemented bool) {
	defer func() {
		if r := recover(); r != nil {
			unimplemented = false
		}
	}()

	out := method.Call(args)
	errv := out[len(out)-1]
	if errv.IsNil() {
		return false
	}
	return status.Code(errv.Interface().(error)) == codes.Unimplemented
}
