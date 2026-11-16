// Command server-descriptors writes the proto descriptors of the services that the proxy carries,
// as a Temporal server release built them, to a gzip FileDescriptorSet. The interceptor tests load
// the set to build messages with an older server schema than the one the proxy is built with.
//
// It is its own module, so that it can require an older go.temporal.io/server than the proxy.
// To write the descriptors for another release, change the versions in go.mod and run:
//
//	cd develop/server-descriptors && go mod tidy && go run . ../../interceptor/testdata/server-<version>-descriptors.binpb.gz
package main

import (
	"compress/gzip"
	"fmt"
	"os"
	"slices"

	_ "go.temporal.io/api/operatorservice/v1"
	_ "go.temporal.io/api/workflowservice/v1"
	_ "go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// services are the gRPC services that the proxy carries. The set holds their files and every file
// that those import.
var services = []protoreflect.FullName{
	"temporal.api.workflowservice.v1.WorkflowService",
	"temporal.api.operatorservice.v1.OperatorService",
	"temporal.server.api.adminservice.v1.AdminService",
}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: server-descriptors <output .binpb.gz>")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(path string) error {
	files := map[string]protoreflect.FileDescriptor{}
	var add func(file protoreflect.FileDescriptor)
	add = func(file protoreflect.FileDescriptor) {
		if _, ok := files[file.Path()]; ok {
			return
		}
		files[file.Path()] = file
		imports := file.Imports()
		for i := range imports.Len() {
			add(imports.Get(i).FileDescriptor)
		}
	}
	for _, name := range services {
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(name)
		if err != nil {
			return err
		}
		add(desc.ParentFile())
	}

	// Write the files in path order, so that the same release always gives the same bytes.
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	set := &descriptorpb.FileDescriptorSet{}
	for _, p := range paths {
		set.File = append(set.File, protodesc.ToFileDescriptorProto(files[p]))
	}
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(set)
	if err != nil {
		return err
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	zw, err := gzip.NewWriterLevel(out, gzip.BestCompression)
	if err != nil {
		return err
	}
	if _, err := zw.Write(data); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	fmt.Printf("wrote %d files to %s\n", len(set.File), path)
	return out.Close()
}
