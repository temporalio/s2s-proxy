package main

import "io"

var (
	lazyClientMap = map[string]string{
		"admin":    "GetAdminClient",
		"frontend": "GetWorkflowServiceClient",
	}
)

func init() {
	ignoreMethod["lazyClient.admin.StreamWorkflowReplicationMessages"] = true
	ignoreMethod["aclServer.admin.StreamWorkflowReplicationMessages"] = true
}

func generateACLServer(w io.Writer, service service) {
	writeTemplatedCode(w, service, `
package {{.ServiceName}}
import (
	"context"
	"{{.ServicePackagePath}}"
	"google.golang.org/grpc"
)
`)

	writeTemplatedMethods(w, service, "aclServer", `
func (s *adminServiceAuth) {{.Method}}(ctx context.Context, in0 {{.RequestType}}) ({{.ResponseType}}, error) {
	if !s.access.IsAllowed("{{.Method}}") {
		return nil, status.Errorf(codes.PermissionDenied, "Calling method {{.Method}} is not allowed.")
	}
	return s.delegate.{{.Method}}(ctx, in0)
}
`)
}

func generateLazyClient(w io.Writer, service service) {
	writeTemplatedCode(w, service, `
package {{.ServiceName}}
import (
	"context"
	"{{.ServicePackagePath}}"
	"google.golang.org/grpc"
)
`)

	writeTemplatedMethods(w, service, "lazyClient", `
func (c *lazyClient) {{.Method}}(
	ctx context.Context,
	request {{.RequestType}},
	opts ...grpc.CallOption,
) ({{.ResponseType}}, error) {
	var resp {{.ResponseType}}
	client, err := c.clientProvider.{{.GetLazyClient}}()
	if err != nil {
		return resp, err
	}
	return client.{{.Method}}(ctx, request, opts...)
}
`)
}
