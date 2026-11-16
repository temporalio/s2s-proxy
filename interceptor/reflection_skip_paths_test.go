package interceptor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// bytesLeafMaxDepth limits how deep findLeaves looks for bytes fields. It is deep enough to reach
// the mutable state inside a stream replication response, which the random builder does not reach.
const bytesLeafMaxDepth = 25

// TestSkippingByteSlicesKeepsVisitorResultsOnEveryBytesField checks every bytes field that a request
// or response of proxiedServices can hold, one at a time, instead of random messages. For each path
// from a root message to a bytes field, it builds a message with only that field set, to a serialized
// history whose events have a namespace and a search attribute that the matchers translate. If the
// field is the data of a DataBlob, the blob is marked as proto3 so that the visitors can decode it.
// Each visitor in skipCheckVisitors must give the same result with and without the byte slice walk.
//
// It also checks what the visitors do with that history, so the comparison is not between two runs
// that both ignore it:
//   - a blob in a field named in dataBlobFieldNames must be translated by the namespace and the
//     search attribute visitors, and
//   - a history in any other bytes field must stay opaque: neither visitor matches it.
func TestSkippingByteSlicesKeepsVisitorResultsOnEveryBytesField(t *testing.T) {
	t.Cleanup(func() { walkByteSlices = false })
	logger := log.NewNoopLogger()
	historyData := translatableHistoryBytes(t)

	var leafCount, blobLeafCount int
	for _, msgType := range proxiedMessageTypes(t) {
		var leaves []leafPath
		findLeaves(msgType.Descriptor(), isBytesField, nil, map[protoreflect.FullName]bool{}, bytesLeafMaxDepth, &leaves)

		for _, leaf := range leaves {
			leafCount++
			isBlob := isTranslatedBlobLeaf(leaf)
			if isBlob {
				blobLeafCount++
			}
			msg := populateBytes(msgType, leaf, historyData).Interface()

			for _, v := range skipCheckVisitors {
				where := []any{"type", msgType.Descriptor().FullName(), "path", leaf.String(), "visitor", v.name}
				walked := runVisitor(logger, v.visit, v.match, msg, true)
				skipped := runVisitor(logger, v.visit, v.match, msg, false)
				require.Equal(t, walked.err == nil, skipped.err == nil,
					"only one run failed: walked=%v skipped=%v %v", walked.err, skipped.err, where)
				if walked.err != nil {
					continue
				}
				require.Equal(t, walked.matched, skipped.matched, "matched differs %v", where)
				require.True(t, proto.Equal(walked.msg, skipped.msg), "message differs %v", where)

				// The access control matcher also matches the empty namespace of an unset field,
				// so only the translating visitors tell whether the history was read.
				if v.name == "access control" {
					continue
				}
				if isBlob {
					require.True(t, skipped.matched, "history blob not translated %v", where)
				} else {
					require.False(t, skipped.matched, "opaque bytes translated %v", where)
				}
			}
		}
	}

	// The roots must reach the stream replication blobs, not only top level fields.
	require.Greater(t, leafCount, 1000, "bytes fields checked")
	require.Greater(t, blobLeafCount, 10, "history blob fields checked")
}

// proxiedMessageTypes returns the request and response types of every method in proxiedServices.
func proxiedMessageTypes(t *testing.T) []protoreflect.MessageType {
	t.Helper()
	var out []protoreflect.MessageType
	seen := map[protoreflect.FullName]bool{}
	for _, serviceName := range proxiedServices {
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(serviceName)
		require.NoError(t, err)
		methods := desc.(protoreflect.ServiceDescriptor).Methods()
		for i := range methods.Len() {
			for _, msgDesc := range []protoreflect.MessageDescriptor{methods.Get(i).Input(), methods.Get(i).Output()} {
				if seen[msgDesc.FullName()] {
					continue
				}
				seen[msgDesc.FullName()] = true
				msgType, err := protoregistry.GlobalTypes.FindMessageByName(msgDesc.FullName())
				require.NoError(t, err)
				out = append(out, msgType)
			}
		}
	}
	return out
}

// isBytesField reports whether f holds bytes, directly, in a list, or as a map value.
func isBytesField(f protoreflect.FieldDescriptor) bool {
	if f.IsMap() {
		f = f.MapValue()
	}
	return f.Kind() == protoreflect.BytesKind
}

// isTranslatedBlobLeaf reports whether leaf ends at the data of a DataBlob held by a field that the
// visitors decode as a history, by its name in dataBlobFieldNames.
func isTranslatedBlobLeaf(leaf leafPath) bool {
	if len(leaf) < 2 {
		return false
	}
	last, blobField := leaf[len(leaf)-1], leaf[len(leaf)-2]
	if last.ContainingMessage().FullName() != dataBlobName {
		return false
	}
	// protoc-gen-go makes the Go field name from the proto field name without the underscores.
	protoName := strings.ReplaceAll(string(blobField.Name()), "_", "")
	for goName := range dataBlobFieldNames {
		if strings.EqualFold(goName, protoName) {
			return true
		}
	}
	return false
}

// populateBytes builds a fresh mt with only the bytes field at path set to data, and returns it.
// If the field is the data of a DataBlob, it also sets the blob encoding to proto3.
func populateBytes(mt protoreflect.MessageType, path leafPath, data []byte) protoreflect.Message {
	root := mt.New()
	m := root
	for _, f := range path[:len(path)-1] {
		m = descend(m, f)
	}

	f := path[len(path)-1]
	value := protoreflect.ValueOfBytes(data)
	switch {
	case f.IsMap():
		m.Mutable(f).Map().Set(mapKey(f.MapKey()), value)
	case f.IsList():
		m.Mutable(f).List().Append(value)
	default:
		m.Set(f, value)
	}
	if m.Descriptor().FullName() == dataBlobName {
		m.Set(m.Descriptor().Fields().ByName("encoding_type"), protoreflect.ValueOfEnum(enums.ENCODING_TYPE_PROTO3.Number()))
	}
	return root
}

// translatableHistoryBytes returns a serialized history whose events have the namespace and the
// search attribute that the skipCheckVisitors matchers translate.
func translatableHistoryBytes(t *testing.T) []byte {
	t.Helper()
	events := &history.History{Events: makeTranslatableEvents("orig", "AudienceId", 16)}
	data, err := proto.Marshal(events)
	require.NoError(t, err)
	return data
}
