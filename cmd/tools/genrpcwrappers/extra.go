package main

import (
	"io"
)

var (
	lazyClientMap = map[string]string{
		"admin":    "GetAdminClient",
		"frontend": "GetWorkflowServiceClient",
	}

	ignoreMethodsNotIn122 = map[string]bool{
		// adminservice methods
		"AddTasks":                            true,
		"CancelDLQJob":                        true,
		"DeepHealthCheck":                     true,
		"DescribeDLQJob":                      true,
		"DescribeTaskQueuePartition":          true,
		"ForceUnloadTaskQueuePartition":       true,
		"GenerateLastHistoryReplicationTasks": true,
		"GetDLQTasks":                         true,
		"GetWorkflowExecutionRawHistory":      true,
		"ImportWorkflowExecution":             true,
		"ListQueues":                          true,
		"MergeDLQTasks":                       true,
		"PurgeDLQTasks":                       true,
		"SyncWorkflowState":                   true,

		// workflowservice
		"DescribeDeployment":             true,
		"ExecuteMultiOperation":          true,
		"GetCurrentDeployment":           true,
		"GetDeploymentReachability":      true,
		"GetWorkerVersioningRules":       true,
		"ListDeployments":                true,
		"PauseActivityById":              true,
		"PollNexusTaskQueue":             true,
		"ResetActivityById":              true,
		"RespondNexusTaskCompleted":      true,
		"RespondNexusTaskFailed":         true,
		"SetCurrentDeployment":           true,
		"ShutdownWorker":                 true,
		"UnpauseActivityById":            true,
		"UpdateActivityOptionsById":      true,
		"UpdateWorkerVersioningRules":    true,
		"UpdateWorkflowExecutionOptions": true,
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

func generate122ConversionCode(w io.Writer, service service) {
	writeTemplatedCode(w, service, `
package compat
import (
	svc "{{.ServicePackagePath}}"
	svc122 "{{.ServicePackagePath122}}"
	"github.com/temporalio/s2s-proxy/common"
)

// {{.ServiceNameTitle}}ConvertTo122 accepts a protobuf type and returns
// the corresponding gogo-based protobuf type from Temporal v1.22.
func {{.ServiceNameTitle}}ConvertTo122(vAny any) (common.Marshaler, bool) {
	switch vAny.(type) {
`)

	writeTemplatedMethods122(w, service, "conversion", `
case *svc.{{.Method}}Response:
	return &svc122.{{.Method}}Response{}, true
case *svc.{{.Method}}Request:
	return &svc122.{{.Method}}Request{}, true
`)

	writeTemplatedCode(w, service, `}
	return nil, false
}
`)
}

func writeTemplatedMethods122(w io.Writer, service service, impl string, text string) {
	sType := service.clientType.Elem()
	for n := 0; n < sType.NumMethod(); n++ {
		m := sType.Method(n)
		if ignoreMethodsNotIn122[m.Name] {
			continue
		}
		writeTemplatedMethod(w, service, impl, m, text)
	}
}
