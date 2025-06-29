// The MIT License
//
// Copyright (c) 2020 Temporal Technologies Inc.  All rights reserved.
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

// Copied and modifed from https://github.com/temporalio/temporal/blob/main/cmd/tools/genrpcwrappers/main.go

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"slices"
	"strings"
	"text/template"

	taskqueuepb "go.temporal.io/api/taskqueue/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/api/historyservice/v1"
	"go.temporal.io/server/api/matchingservice/v1"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type (
	service struct {
		name            string
		clientType      reflect.Type
		clientGenerator func(io.Writer, service)
	}

	fieldWithPath struct {
		field *reflect.StructField
		path  string
	}
)

func (f fieldWithPath) found() bool {
	return f.path != ""
}

var (
	services = []service{
		{
			name:            "frontend",
			clientType:      reflect.TypeOf((*workflowservice.WorkflowServiceClient)(nil)),
			clientGenerator: generateFrontendOrAdminClient,
		},
		{
			name:            "admin",
			clientType:      reflect.TypeOf((*adminservice.AdminServiceClient)(nil)),
			clientGenerator: generateFrontendOrAdminClient,
		},
		{
			name:            "history",
			clientType:      reflect.TypeOf((*historyservice.HistoryServiceClient)(nil)),
			clientGenerator: generateHistoryClient,
		},
		{
			name:            "matching",
			clientType:      reflect.TypeOf((*matchingservice.MatchingServiceClient)(nil)),
			clientGenerator: generateMatchingClient,
		},
	}

	longPollContext = map[string]bool{
		"client.frontend.ListArchivedWorkflowExecutions": true,
		"client.frontend.PollActivityTaskQueue":          true,
		"client.frontend.PollWorkflowTaskQueue":          true,
		"client.matching.GetTaskQueueUserData":           true,
		"client.matching.ListNexusEndpoints":             true,
	}
	largeTimeoutContext = map[string]bool{
		"client.admin.GetReplicationMessages": true,
	}
	ignoreMethod = map[string]bool{
		// TODO stream APIs are not supported. do not generate.
		"client.admin.StreamWorkflowReplicationMessages":          true,
		"metricsClient.admin.StreamWorkflowReplicationMessages":   true,
		"retryableClient.admin.StreamWorkflowReplicationMessages": true,
		// TODO(bergundy): Allow specifying custom routing for streaming messages.
		"client.history.StreamWorkflowReplicationMessages":          true,
		"metricsClient.history.StreamWorkflowReplicationMessages":   true,
		"retryableClient.history.StreamWorkflowReplicationMessages": true,

		// these need to pick a partition. too complicated.
		"client.matching.AddActivityTask":       true,
		"client.matching.AddWorkflowTask":       true,
		"client.matching.PollActivityTaskQueue": true,
		"client.matching.PollWorkflowTaskQueue": true,
		"client.matching.QueryWorkflow":         true,
		// these do forwarding stats. too complicated.
		"metricsClient.matching.AddActivityTask":       true,
		"metricsClient.matching.AddWorkflowTask":       true,
		"metricsClient.matching.PollActivityTaskQueue": true,
		"metricsClient.matching.PollWorkflowTaskQueue": true,
		"metricsClient.matching.QueryWorkflow":         true,
	}
	// Fields to ignore when looking for the routing fields in a request object.
	ignoreField = map[string]bool{
		// this is the workflow that sent a signal
		"SignalWorkflowExecutionRequest.ExternalWorkflowExecution": true,
		// this is the workflow that sent a cancel request
		"RequestCancelWorkflowExecutionRequest.ExternalWorkflowExecution": true,
		// this is the workflow that sent a terminate
		"TerminateWorkflowExecutionRequest.ExternalWorkflowExecution": true,
		// this is the parent for starting a child workflow
		"StartWorkflowExecutionRequest.ParentExecutionInfo": true,
		// this is the root for starting a child workflow
		"StartWorkflowExecutionRequest.RootExecutionInfo": true,
		// these get routed to the parent
		"RecordChildExecutionCompletedRequest.ChildExecution":          true,
		"VerifyChildExecutionCompletionRecordedRequest.ChildExecution": true,
	}
)

var historyRoutingProtoExtension = func() protoreflect.ExtensionType {
	ext, err := protoregistry.GlobalTypes.FindExtensionByName("temporal.server.api.historyservice.v1.routing")
	if err != nil {
		log.Fatalf("Error finding extension: %s", err)
	}
	return ext
}()

func panicIfErr(err error) {
	if err != nil {
		panic(err)
	}
}

func writeTemplatedCode(w io.Writer, service service, text string) {
	panicIfErr(template.Must(template.New("code").Parse(text)).Execute(w, map[string]string{
		"ServiceName":        service.name,
		"ServicePackagePath": service.clientType.Elem().PkgPath(),
	}))
}

func verifyFieldExists(t reflect.Type, path string) {
	pathPrefix := t.String()
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if t.Kind() != reflect.Struct {
			panic(fmt.Errorf("%s is not a struct", pathPrefix))
		}
		fieldName := snakeToPascal(part)
		f, ok := t.FieldByName(fieldName)
		if !ok {
			panic(fmt.Errorf("%s has no field named %s", pathPrefix, fieldName))
		}
		if i == len(parts)-1 {
			return
		}
		ft := f.Type
		if ft.Kind() != reflect.Pointer {
			panic(fmt.Errorf("%s.%s is not a struct pointer", pathPrefix, fieldName))
		}
		t = ft.Elem()
		pathPrefix += "." + fieldName
	}
}

func findNestedField(t reflect.Type, name string, path string, maxDepth int) []fieldWithPath {
	if t.Kind() != reflect.Struct || maxDepth <= 0 {
		return nil
	}
	var out []fieldWithPath
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if ignoreField[t.Name()+"."+f.Name] {
			continue
		}
		if f.Name == name {
			out = append(out, fieldWithPath{field: &f, path: path + ".Get" + name + "()"})
		}
		ft := f.Type
		if ft.Kind() == reflect.Pointer {
			out = append(out, findNestedField(ft.Elem(), name, path+".Get"+f.Name+"()", maxDepth-1)...)
		}
	}
	return out
}

func findOneNestedField(t reflect.Type, name string, path string, maxDepth int) fieldWithPath {
	fields := findNestedField(t, name, path, maxDepth)
	if len(fields) == 0 {
		panic(fmt.Sprintf("Couldn't find %s in %s", name, t))
	} else if len(fields) > 1 {
		panic(fmt.Sprintf("Found more than one %s in %s (%v)", name, t, fields))
	}
	return fields[0]
}

func tryFindOneNestedField(t reflect.Type, name string, path string, maxDepth int) fieldWithPath {
	fields := findNestedField(t, name, path, maxDepth)
	if len(fields) == 0 {
		return fieldWithPath{}
	} else if len(fields) > 1 {
		panic(fmt.Sprintf("Found more than one %s in %s (%v)", name, t, fields))
	}
	return fields[0]
}

func historyRoutingOptions(reqType reflect.Type) *historyservice.RoutingOptions {
	t := reqType.Elem() // we know it's a pointer

	inst := reflect.New(t)
	reflectable, ok := inst.Interface().(interface{ ProtoReflect() protoreflect.Message })
	if !ok {
		log.Fatalf("Request has no ProtoReflect method %s", t)
	}
	opts := reflectable.ProtoReflect().Descriptor().Options()

	// Retrieve the value of the custom option
	optionValue := proto.GetExtension(opts, historyRoutingProtoExtension)
	if optionValue == nil {
		log.Fatalf("Got nil while retrieving extension from options")
	}

	routingOptions := optionValue.(*historyservice.RoutingOptions)
	if routingOptions == nil {
		log.Fatalf("Request has no routing options: %s", t)
	}
	return routingOptions
}

func snakeToPascal(snake string) string {
	// Split the string by underscores
	words := strings.Split(snake, "_")

	// Capitalize the first letter of each word
	for i, word := range words {
		// Convert first rune to upper and the rest to lower case
		words[i] = cases.Title(language.AmericanEnglish).String(strings.ToLower(word))
	}

	// Join them back into a single string
	return strings.Join(words, "")
}

func toGetter(snake string) string {
	parts := strings.Split(snake, ".")
	for i, part := range parts {
		parts[i] = "Get" + snakeToPascal(part) + "()"
	}
	return "request." + strings.Join(parts, ".")
}

func makeGetHistoryClient(reqType reflect.Type, routingOptions *historyservice.RoutingOptions) string {
	t := reqType.Elem() // we know it's a pointer

	if routingOptions.AnyHost && routingOptions.ShardId != "" && routingOptions.WorkflowId != "" && routingOptions.TaskToken != "" && routingOptions.TaskInfos != "" {
		log.Fatalf("Found more than one routing directive in %s", t)
	}
	if routingOptions.AnyHost {
		return "shardID := c.getRandomShard()"
	}
	if routingOptions.ShardId != "" {
		verifyFieldExists(t, routingOptions.ShardId)
		return "shardID := " + toGetter(routingOptions.ShardId)
	}
	if routingOptions.WorkflowId != "" {
		namespaceIdField := routingOptions.NamespaceId
		if namespaceIdField == "" {
			namespaceIdField = "namespace_id"
		}
		verifyFieldExists(t, namespaceIdField)
		verifyFieldExists(t, routingOptions.WorkflowId)
		return fmt.Sprintf("shardID := c.shardIDFromWorkflowID(%s, %s)", toGetter(namespaceIdField), toGetter(routingOptions.WorkflowId))
	}
	if routingOptions.TaskToken != "" {
		namespaceIdField := routingOptions.NamespaceId
		if namespaceIdField == "" {
			namespaceIdField = "namespace_id"
		}

		verifyFieldExists(t, namespaceIdField)
		verifyFieldExists(t, routingOptions.TaskToken)
		return fmt.Sprintf(`taskToken, err := c.tokenSerializer.Deserialize(%s)
	if err != nil {
		return nil, serviceerror.NewInvalidArgument("error deserializing task token")
	}
	shardID := c.shardIDFromWorkflowID(%s, taskToken.GetWorkflowId())
`, toGetter(routingOptions.TaskToken), toGetter(namespaceIdField))
	}
	if routingOptions.TaskInfos != "" {
		verifyFieldExists(t, routingOptions.TaskInfos)
		p := toGetter(routingOptions.TaskInfos)
		// slice needs a tiny bit of extra handling for namespace
		return fmt.Sprintf(`// All workflow IDs are in the same shard per request
	if len(%s) == 0 {
		return nil, serviceerror.NewInvalidArgument("missing TaskInfos")
	}
	shardID := c.shardIDFromWorkflowID(%s[0].NamespaceId, %s[0].WorkflowId)`, p, p, p)
	}

	log.Fatalf("No routing directive specified on %s", t)
	panic("unreachable")
}

func makeGetMatchingClient(reqType reflect.Type) string {
	// this magically figures out how to get a MatchingServiceClient from a request
	t := reqType.Elem() // we know it's a pointer

	var nsID, tqp, tq, tqt fieldWithPath

	switch t.Name() {
	case "GetBuildIdTaskQueueMappingRequest":
		// Pick a random node for this request, it's not associated with a specific task queue.
		tq = fieldWithPath{path: "fmt.Sprintf(\"not-applicable-%d\", rand.Int())"}
		tqt = fieldWithPath{path: "enumspb.TASK_QUEUE_TYPE_UNSPECIFIED"}
		nsID = findOneNestedField(t, "NamespaceId", "request", 1)
	case "UpdateTaskQueueUserDataRequest",
		"ReplicateTaskQueueUserDataRequest":
		// Always route these requests to the same matching node by namespace.
		tq = fieldWithPath{path: "\"not-applicable\""}
		tqt = fieldWithPath{path: "enumspb.TASK_QUEUE_TYPE_UNSPECIFIED"}
		nsID = findOneNestedField(t, "NamespaceId", "request", 1)
	case "GetWorkerBuildIdCompatibilityRequest",
		"UpdateWorkerBuildIdCompatibilityRequest",
		"RespondQueryTaskCompletedRequest",
		"ListTaskQueuePartitionsRequest",
		"SyncDeploymentUserDataRequest",
		"CheckTaskQueueUserDataPropagationRequest",
		"ApplyTaskQueueUserDataReplicationEventRequest",
		"GetWorkerVersioningRulesRequest",
		"UpdateWorkerVersioningRulesRequest":
		tq = findOneNestedField(t, "TaskQueue", "request", 2)
		tqt = fieldWithPath{path: "enumspb.TASK_QUEUE_TYPE_WORKFLOW"}
		nsID = findOneNestedField(t, "NamespaceId", "request", 1)
	case "DispatchNexusTaskRequest",
		"PollNexusTaskQueueRequest",
		"RespondNexusTaskCompletedRequest",
		"RespondNexusTaskFailedRequest":
		tq = findOneNestedField(t, "TaskQueue", "request", 2)
		tqt = fieldWithPath{path: "enumspb.TASK_QUEUE_TYPE_NEXUS"}
		nsID = findOneNestedField(t, "NamespaceId", "request", 1)
	case "CreateNexusEndpointRequest",
		"UpdateNexusEndpointRequest",
		"ListNexusEndpointsRequest",
		"DeleteNexusEndpointRequest":
		// Always route these requests to the same matching node for all namespaces.
		tq = fieldWithPath{path: `"not-applicable"`}
		tqt = fieldWithPath{path: "enumspb.TASK_QUEUE_TYPE_UNSPECIFIED"}
		nsID = fieldWithPath{path: `"not-applicable"`}
	default:
		tqp = tryFindOneNestedField(t, "TaskQueuePartition", "request", 1)
		tq = findOneNestedField(t, "TaskQueue", "request", 2)
		tqt = findOneNestedField(t, "TaskQueueType", "request", 2)
		nsID = findOneNestedField(t, "NamespaceId", "request", 1)
	}

	if !nsID.found() {
		panic("I don't know how to get a client from a " + t.String())
	}

	if tqp.found() {
		return fmt.Sprintf(
			`p := tqid.PartitionFromPartitionProto(%s, %s)

	client, err := c.getClientForTaskQueuePartition(p)`,
			tqp.path, nsID.path)
	}

	if tq.found() && tqt.found() {
		partitionMaker := fmt.Sprintf("tqid.PartitionFromProto(%s, %s, %s)", tq.path, nsID.path, tqt.path)
		// Some task queue fields are full messages, some are just strings
		isTaskQueueMessage := tq.field != nil && tq.field.Type == reflect.TypeOf((*taskqueuepb.TaskQueue)(nil))
		if !isTaskQueueMessage {
			partitionMaker = fmt.Sprintf("tqid.NormalPartitionFromRpcName(%s, %s, %s)", tq.path, nsID.path, tqt.path)
		}

		return fmt.Sprintf(
			`p, err := %s
	if err != nil {
		return nil, err
	}

	client, err := c.getClientForTaskQueuePartition(p)`,
			partitionMaker)
	}

	panic("I don't know how to get a client from a " + t.String())
}

func writeTemplatedMethod(w io.Writer, service service, impl string, m reflect.Method, text string) {
	key := fmt.Sprintf("%s.%s.%s", impl, service.name, m.Name)
	if ignoreMethod[key] {
		return
	}

	mt := m.Type // should look like: func(context.Context, request reqType, opts []grpc.CallOption) (respType, error)
	if !mt.IsVariadic() ||
		mt.NumIn() != 3 ||
		mt.NumOut() != 2 ||
		mt.In(0).String() != "context.Context" ||
		mt.Out(1).String() != "error" {
		panic(key + " doesn't look like a grpc handler method")
	}

	reqType := mt.In(1)
	respType := mt.Out(0)

	fields := map[string]string{
		"Method":       m.Name,
		"RequestType":  reqType.String(),
		"ResponseType": respType.String(),
		"MetricPrefix": fmt.Sprintf("%s%sClient", strings.ToUpper(service.name[:1]), service.name[1:]),
		// s2s-proxy customization
		"GetLazyClient": lazyClientMap[service.name],
	}
	if longPollContext[key] {
		fields["LongPoll"] = "LongPoll"
	}
	if largeTimeoutContext[key] {
		fields["WithLargeTimeout"] = "WithLargeTimeout"
	}
	if impl == "client" {
		switch service.name {
		case "history":
			routingOptions := historyRoutingOptions(reqType)
			if routingOptions.Custom {
				return
			}
			fields["GetClient"] = makeGetHistoryClient(reqType, routingOptions)
		case "matching":
			fields["GetClient"] = makeGetMatchingClient(reqType)
		}
	}

	panicIfErr(template.Must(template.New("code").Parse(text)).Execute(w, fields))
}

func writeTemplatedMethods(w io.Writer, service service, impl string, text string) {
	sType := service.clientType.Elem()
	for n := 0; n < sType.NumMethod(); n++ {
		writeTemplatedMethod(w, service, impl, sType.Method(n), text)
	}
}

func generateFrontendOrAdminClient(w io.Writer, service service) {
	writeTemplatedCode(w, service, `
package {{.ServiceName}}

import (
	"context"

	"{{.ServicePackagePath}}"
	"google.golang.org/grpc"
)
`)

	writeTemplatedMethods(w, service, "client", `
func (c *clientImpl) {{.Method}}(
	ctx context.Context,
	request {{.RequestType}},
	opts ...grpc.CallOption,
) ({{.ResponseType}}, error) {
	ctx, cancel := c.create{{or .LongPoll ""}}Context{{or .WithLargeTimeout ""}}(ctx)
	defer cancel()
	return c.client.{{.Method}}(ctx, request, opts...)
}
`)
}

func generateHistoryClient(w io.Writer, service service) {
	writeTemplatedCode(w, service, `
package {{.ServiceName}}

import (
	"context"

	"go.temporal.io/api/serviceerror"
	"{{.ServicePackagePath}}"
	"google.golang.org/grpc"
)
`)

	writeTemplatedMethods(w, service, "client", `
func (c *clientImpl) {{.Method}}(
	ctx context.Context,
	request {{.RequestType}},
	opts ...grpc.CallOption,
) ({{.ResponseType}}, error) {
	{{.GetClient}}
	var response {{.ResponseType}}
	op := func(ctx context.Context, client historyservice.HistoryServiceClient) error {
		var err error
		ctx, cancel := c.createContext(ctx)
		defer cancel()
		response, err = client.{{.Method}}(ctx, request, opts...)
		return err
	}
	if err := c.executeWithRedirect(ctx, shardID, op); err != nil {
		return nil, err
	}
	return response, nil
}
`)
	// TODO: some methods call client.{{.Method}} directly and do not use executeWithRedirect. should we preserve this?
	// GetDLQReplicationMessages
	// GetDLQMessages
	// PurgeDLQMessages
	// MergeDLQMessages
}

func generateMatchingClient(w io.Writer, service service) {
	writeTemplatedCode(w, service, `
package {{.ServiceName}}

import (
	"context"
	"fmt"
	"math/rand"

	enumspb "go.temporal.io/api/enums/v1"
	"{{.ServicePackagePath}}"
	"go.temporal.io/server/common/tqid"
	"google.golang.org/grpc"
)
`)

	writeTemplatedMethods(w, service, "client", `
func (c *clientImpl) {{.Method}}(
	ctx context.Context,
	request {{.RequestType}},
	opts ...grpc.CallOption,
) ({{.ResponseType}}, error) {

	{{.GetClient}}
	if err != nil {
		return nil, err
	}
	ctx, cancel := c.create{{or .LongPoll ""}}Context(ctx)
	defer cancel()
	return client.{{.Method}}(ctx, request, opts...)
}
`)
}

func generateMetricClient(w io.Writer, service service) {
	writeTemplatedCode(w, service, `
package {{.ServiceName}}

import (
	"context"

	"{{.ServicePackagePath}}"
	"google.golang.org/grpc"
)
`)

	writeTemplatedMethods(w, service, "metricsClient", `
func (c *metricClient) {{.Method}}(
	ctx context.Context,
	request {{.RequestType}},
	opts ...grpc.CallOption,
) (_ {{.ResponseType}}, retError error) {

	metricsHandler, startTime := c.startMetricsRecording(ctx, "{{.MetricPrefix}}{{.Method}}")
	defer func() {
		c.finishMetricsRecording(metricsHandler, startTime, retError)
	}()

	return c.client.{{.Method}}(ctx, request, opts...)
}
`)
}

func generateRetryableClient(w io.Writer, service service) {
	writeTemplatedCode(w, service, `
package {{.ServiceName}}

import (
	"context"

	"{{.ServicePackagePath}}"
	"google.golang.org/grpc"

	"go.temporal.io/server/common/backoff"
)
`)

	writeTemplatedMethods(w, service, "retryableClient", `
func (c *retryableClient) {{.Method}}(
	ctx context.Context,
	request {{.RequestType}},
	opts ...grpc.CallOption,
) ({{.ResponseType}}, error) {
	var resp {{.ResponseType}}
	op := func(ctx context.Context) error {
		var err error
		resp, err = c.client.{{.Method}}(ctx, request, opts...)
		return err
	}
	err := backoff.ThrottleRetryContext(ctx, op, c.policy, c.isRetryable)
	return resp, err
}
`)
}

func callWithFile(f func(io.Writer, service), service service, filename string, licenseText string) {
	w, err := os.Create(filename + "_gen.go")
	if err != nil {
		panic(err)
	}
	defer func() {
		panicIfErr(w.Close())
	}()
	if _, err := fmt.Fprintf(w, "%s\n// Code generated by cmd/tools/genrpcwrappers. DO NOT EDIT.\n", licenseText); err != nil {
		panic(err)
	}
	f(w, service)
}

func readLicenseFile(path string) string {
	text, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var lines []string
	for _, line := range strings.Split(string(text), "\n") {
		lines = append(lines, strings.TrimRight("// "+line, " "))
	}
	return strings.Join(lines, "\n") + "\n"
}

func main() {
	serviceFlag := flag.String("service", "", "which service to generate rpc client wrappers for")
	licenseFlag := flag.String("license_file", "../../LICENSE", "path to license to copy into header")
	flag.Parse()

	i := slices.IndexFunc(services, func(s service) bool { return s.name == *serviceFlag })
	if i < 0 {
		panic("unknown service")
	}
	svc := services[i]

	licenseText := readLicenseFile(*licenseFlag)

	callWithFile(svc.clientGenerator, svc, "client", licenseText)
	callWithFile(generateMetricClient, svc, "metric_client", licenseText)
	callWithFile(generateRetryableClient, svc, "retryable_client", licenseText)

	// s2s-proxy customizations
	if svc.name == "admin" || svc.name == "frontend" {
		callWithFile(generateLazyClient, svc, "lazy_client", "")
	}
	if svc.name == "admin" {
		callWithFile(generateACLServer, svc, "acl_server", "")
	}
}
