package main

import (
	"fmt"
	"io"

	"google.golang.org/protobuf/reflect/protoreflect"
)

const (
	publicAPIPrefix                       = "temporal.api."
	dataBlobMessage protoreflect.FullName = "temporal.api.common.v1.DataBlob"
	anyMessage      protoreflect.FullName = "google.protobuf.Any"
)

func writeln(out io.Writer, args ...any) {
	if _, err := fmt.Fprintln(out, args...); err != nil {
		panic(err)
	}
}

func writef(out io.Writer, msg string, args ...any) {
	if _, err := fmt.Fprintf(out, msg, args...); err != nil {
		panic(err)
	}
}

func keys[V any](m map[protoreflect.FullName]V) []protoreflect.FullName {
	out := make([]protoreflect.FullName, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
