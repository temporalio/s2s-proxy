package interceptor

import (
	"compress/gzip"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

// server128Descriptors is the descriptor set of the services that the proxy carries, as Temporal
// server v1.28.4 built them. develop/server-descriptors writes it.
const server128Descriptors = "testdata/server-1.28.4-descriptors.binpb.gz"

// loadSchema reads a gzip FileDescriptorSet from path and returns its schema. The types are
// dynamic, and are not in the global registry, so they do not conflict with the proxy types.
func loadSchema(t *testing.T, name, path string) *testSchema {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	zr, err := gzip.NewReader(f)
	require.NoError(t, err)
	data, err := io.ReadAll(zr)
	require.NoError(t, err)

	var set descriptorpb.FileDescriptorSet
	require.NoError(t, proto.Unmarshal(data, &set))
	files, err := protodesc.NewFiles(&set)
	require.NoError(t, err)
	types := dynamicpb.NewTypes(files)
	historyType, err := types.FindMessageByName(historyName)
	require.NoError(t, err)
	historyEventType, err := types.FindMessageByName(proxySchema.historyEvent.Descriptor().FullName())
	require.NoError(t, err)
	return &testSchema{name: name, history: historyType, historyEvent: historyEventType, files: files, types: types}
}

// TestSkippingByteSlicesKeepsVisitorResultsWithServer128Schema runs the byte slice skip check on
// messages of the Temporal server v1.28 schema, as a v1.28 server sends them to the proxy. It builds
// every type that a proxied request or response, or a history blob, can hold in that schema, like
// TestSkippingByteSlicesKeepsVisitorResultsInEveryMessageType does for the proxy schema. The history
// in each blob also has v1.28 events. The proxy decodes the bytes of a message into its own type of
// the same name, so the test encodes each message and decodes it into the proxy type before it runs
// the visitors. A field that the proxy type does not have stays as unknown bytes, which the visitors
// do not walk.
//
// It also checks that:
//   - no bytes field of the v1.28 schema has a visitor field name, as
//     TestNoBytesFieldHasAVisitorFieldName checks for the proxy schema,
//   - each field that the visitors translate holds a translatable value in at least one message, and
//   - for each type, each visitor that a field of both schemas has a rule for matches in at least
//     one message.
func TestSkippingByteSlicesKeepsVisitorResultsWithServer128Schema(t *testing.T) {
	t.Cleanup(func() { walkByteSlices = false })
	logger := log.NewNoopLogger()
	schema := loadSchema(t, "server v1.28.4", server128Descriptors)
	types := reachableMessageTypes(t, schema)
	require.Empty(t, bytesFieldsWithVisitorFieldNames(types), "bytes fields with a visitor field name")

	var ruleFields []protoreflect.FieldDescriptor
	var notInProxy []protoreflect.FullName
	cov := newTestCoverage()
	for _, msgType := range types {
		desc := msgType.Descriptor()
		_, fields := visitorRules(desc)
		ruleFields = append(ruleFields, fields...)
		proxyType, err := protoregistry.GlobalTypes.FindMessageByName(desc.FullName())
		if err != nil {
			// The proxy cannot decode a type it does not have, so no proxy request holds it.
			notInProxy = append(notInProxy, desc.FullName())
			continue
		}

		matched := map[string]bool{}
		for variant := range variantsFor(desc) {
			msg := newTestMessage(schema, msgType, variant, cov)
			proxyMsg := decodeAsProxyType(t, msg, proxyType)
			for _, visitorName := range requireSameVisitorResults(t, logger, proxyMsg, "schema", schema.name, "variant", variant) {
				matched[visitorName] = true
			}
		}
		rules := sharedVisitorRules(desc, proxyType.Descriptor())
		if rules[namespaceRule] || rules[dataBlobRule] {
			require.True(t, matched["namespace"], "the namespace visitor matched no %s message", desc.FullName())
		}
		if rules[searchAttributeRule] || rules[dataBlobRule] {
			require.True(t, matched["search attribute"], "the search attribute visitor matched no %s message", desc.FullName())
		}
	}

	for _, field := range ruleFields {
		require.True(t, cov.translatable[field.FullName()], "no message set %s to a value that a test matcher translates", field.FullName())
	}

	summary := cov.summary(types)
	t.Logf("coverage of %s: %v; types the proxy does not have %d: %v", schema.name, summary, len(notInProxy), notInProxy)
	require.Equal(t, summary.oneofChoices, summary.oneofChoicesMade, "oneof choices made")
	require.Equal(t, summary.stringFields, summary.stringFieldsWithEveryValue, "string fields with every test string")
	require.Greater(t, len(types), 500, "message types checked")
}

// decodeAsProxyType encodes msg and decodes the bytes into a new message of proxyType, as the
// proxy decodes a message that a server sends.
func decodeAsProxyType(t *testing.T, msg proto.Message, proxyType protoreflect.MessageType) proto.Message {
	t.Helper()
	data, err := proto.Marshal(msg)
	require.NoError(t, err)
	out := proxyType.New().Interface()
	require.NoError(t, proto.Unmarshal(data, out), "decode %s as the proxy type", proxyType.Descriptor().FullName())
	return out
}

// sharedVisitorRules returns the rules that the visitors apply to the fields of desc that the proxy
// type proxyDesc also has, with the same number and the same rule. Only those fields reach the
// visitors after the proxy decodes a message.
func sharedVisitorRules(desc, proxyDesc protoreflect.MessageDescriptor) map[visitorRule]bool {
	rules := map[visitorRule]bool{}
	fields := desc.Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		rule, ok := visitorRuleOf(field)
		if !ok {
			continue
		}
		proxyField := proxyDesc.Fields().ByNumber(field.Number())
		if proxyField == nil {
			continue
		}
		if proxyRule, ok := visitorRuleOf(proxyField); ok && proxyRule == rule {
			rules[rule] = true
		}
	}
	return rules
}

// bytesFieldsWithVisitorFieldNames returns each bytes field of types, not in a list or a map, with
// a name from namespaceFieldNames, dataBlobFieldNames or searchAttributeFieldNames. The visitors
// skip a byte slice before they look at its field name, so such a field would not get its rule.
func bytesFieldsWithVisitorFieldNames(types []protoreflect.MessageType) []protoreflect.FullName {
	var found []protoreflect.FullName
	for _, msgType := range types {
		fields := msgType.Descriptor().Fields()
		for i := range fields.Len() {
			field := fields.Get(i)
			if field.Kind() != protoreflect.BytesKind || field.IsList() || field.IsMap() {
				continue
			}
			name := lowerGoName(field)
			if lowerNamespaceFieldNames[name] || lowerDataBlobFieldNames[name] || lowerSearchAttributeFieldNames[name] {
				found = append(found, field.FullName())
			}
		}
	}
	return found
}
