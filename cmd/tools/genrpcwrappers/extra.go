package main

import "io"

var (
	lazyClientMap = map[string]string{
		"admin":    "GetAdminClient",
		"operator": "GetOperatorClient",
		"frontend": "GetWorkflowServiceClient",
	}
)

func init() {
	ignoreMethod["lazyClient.admin.StreamWorkflowReplicationMessages"] = true
	ignoreMethod["aclServer.admin.StreamWorkflowReplicationMessages"] = true
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
