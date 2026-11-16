package interceptor

import (
	"math/rand/v2"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/api/history/v1"
	"go.temporal.io/server/common/log"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"

	"github.com/temporalio/s2s-proxy/auth"
)

// proxiedServices are the gRPC services whose requests and responses go through the visitors.
var proxiedServices = []protoreflect.FullName{
	"temporal.api.workflowservice.v1.WorkflowService",
	"temporal.api.operatorservice.v1.OperatorService",
	"temporal.server.api.adminservice.v1.AdminService",
}

// randomMessagesPerType is the number of random messages for each request and response type.
const randomMessagesPerType = 50

// TestSkippingByteSlicesKeepsVisitorResults checks that skipping byte slices only saves work.
// For random requests and responses of every method in proxiedServices, it runs each visitor
// once with the byte slice walk and once without it. Both runs must return the same message and
// the same matched value, or both must return an error. After an error the message is only
// partly translated and its content is not defined, so the test then only checks that both fail.
func TestSkippingByteSlicesKeepsVisitorResults(t *testing.T) {
	t.Cleanup(func() { walkByteSlices = false })
	logger := log.NewNoopLogger()
	visitors := []struct {
		name  string
		visit visitor
		match stringMatcher
	}{
		{name: "namespace", visit: visitNamespace, match: createStringMatcher(map[string]string{"orig": "orig.cloud"})},
		{name: "search attribute", visit: visitSearchAttributes, match: createStringMatcher(map[string]string{"AudienceId": "Keyword01"})},
		{name: "access control", visit: visitNamespace, match: createNamespaceAccessControl(auth.NewAccesControl([]string{"orig"}))},
	}

	for _, serviceName := range proxiedServices {
		desc, err := protoregistry.GlobalFiles.FindDescriptorByName(serviceName)
		require.NoError(t, err)
		methods := desc.(protoreflect.ServiceDescriptor).Methods()
		for i := range methods.Len() {
			method := methods.Get(i)
			for _, msgDesc := range []protoreflect.MessageDescriptor{method.Input(), method.Output()} {
				msgType, err := protoregistry.GlobalTypes.FindMessageByName(msgDesc.FullName())
				require.NoError(t, err)
				for seed := range uint64(randomMessagesPerType) {
					msg := newRandomMessage(msgType, seed)
					for _, v := range visitors {
						walked := runVisitor(logger, v.visit, v.match, msg, true)
						skipped := runVisitor(logger, v.visit, v.match, msg, false)
						where := []any{"method", method.FullName(), "type", msgDesc.FullName(), "visitor", v.name, "seed", seed}
						require.Equal(t, walked.err == nil, skipped.err == nil,
							"only one run failed: walked=%v skipped=%v %v", walked.err, skipped.err, where)
						if walked.err != nil {
							continue
						}
						require.Equal(t, walked.matched, skipped.matched, "matched differs %v", where)
						require.True(t, proto.Equal(walked.msg, skipped.msg), "message differs %v", where)
					}
				}
			}
		}
	}
}

type visitorResult struct {
	msg     proto.Message
	matched bool
	err     error
}

// runVisitor runs the visitor on a copy of msg, with or without the byte slice walk.
func runVisitor(logger log.Logger, visit visitor, match stringMatcher, msg proto.Message, walk bool) visitorResult {
	walkByteSlices = walk
	out := proto.Clone(msg)
	matched, err := visit(logger, out, match)
	normalizeHistoryBlobs(out.ProtoReflect())
	return visitorResult{msg: out, matched: matched, err: err}
}

// normalizeHistoryBlobs encodes each data blob that holds a history again with a fixed map order.
// A translated blob is encoded again with random map order, so equal events can give different bytes.
func normalizeHistoryBlobs(m protoreflect.Message) {
	if m.Descriptor().FullName() == dataBlobName {
		dataField := m.Descriptor().Fields().ByName("data")
		var events history.History
		if err := proto.Unmarshal(m.Get(dataField).Bytes(), &events); err != nil {
			return
		}
		normalizeHistoryBlobs(events.ProtoReflect())
		data, err := proto.MarshalOptions{Deterministic: true}.Marshal(&events)
		if err == nil {
			m.Set(dataField, protoreflect.ValueOfBytes(data))
		}
		return
	}
	m.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		switch {
		case field.IsMap():
			if field.MapValue().Kind() == protoreflect.MessageKind {
				value.Map().Range(func(_ protoreflect.MapKey, v protoreflect.Value) bool {
					normalizeHistoryBlobs(v.Message())
					return true
				})
			}
		case field.IsList():
			if field.Kind() == protoreflect.MessageKind {
				for i := range value.List().Len() {
					normalizeHistoryBlobs(value.List().Get(i).Message())
				}
			}
		case field.Kind() == protoreflect.MessageKind:
			normalizeHistoryBlobs(value.Message())
		}
		return true
	})
}

var dataBlobName = (&common.DataBlob{}).ProtoReflect().Descriptor().FullName()

// randomStrings holds the names that the test matchers translate, and names that they do not.
var randomStrings = []string{"orig", "other", "AudienceId", ""}

// randomMessageMaxDepth limits how deep the builder fills nested messages.
const randomMessageMaxDepth = 5

// randomMessageBuilder fills a message with random values. Most data blobs, and some other byte
// fields, hold a serialized history of random events, so the visitors also translate inside blobs.
type randomMessageBuilder struct {
	rand *rand.Rand
}

func newRandomMessage(msgType protoreflect.MessageType, seed uint64) proto.Message {
	b := &randomMessageBuilder{rand: rand.New(rand.NewPCG(seed, 0))}
	m := msgType.New()
	b.fill(m, 0)
	return m.Interface()
}

func (b *randomMessageBuilder) fill(m protoreflect.Message, depth int) {
	desc := m.Descriptor()
	if desc.FullName() == dataBlobName && b.rand.IntN(10) < 7 {
		m.Set(desc.Fields().ByName("encoding_type"), protoreflect.ValueOfEnum(enums.ENCODING_TYPE_PROTO3.Number()))
		m.Set(desc.Fields().ByName("data"), protoreflect.ValueOfBytes(b.historyBytes(depth)))
		return
	}
	filledOneofs := map[protoreflect.FullName]bool{}
	fields := desc.Fields()
	for i := range fields.Len() {
		field := fields.Get(i)
		if oneof := field.ContainingOneof(); oneof != nil && !oneof.IsSynthetic() {
			if filledOneofs[oneof.FullName()] {
				continue
			}
			filledOneofs[oneof.FullName()] = true
			if b.rand.IntN(3) == 0 {
				continue
			}
			field = oneof.Fields().Get(b.rand.IntN(oneof.Fields().Len()))
		} else if b.rand.IntN(4) == 0 {
			continue
		}
		b.setField(m, field, depth)
	}
}

func (b *randomMessageBuilder) setField(m protoreflect.Message, field protoreflect.FieldDescriptor, depth int) {
	switch {
	case field.IsMap():
		if depth >= randomMessageMaxDepth {
			return
		}
		entries := m.Mutable(field).Map()
		for range b.rand.IntN(3) {
			key := b.scalar(field.MapKey(), depth).MapKey()
			if isMessageField(field.MapValue()) {
				value := entries.NewValue()
				b.fill(value.Message(), depth+1)
				entries.Set(key, value)
			} else {
				entries.Set(key, b.scalar(field.MapValue(), depth))
			}
		}
	case field.IsList():
		if isMessageField(field) && depth >= randomMessageMaxDepth {
			return
		}
		list := m.Mutable(field).List()
		for range b.rand.IntN(3) {
			if isMessageField(field) {
				element := list.NewElement()
				b.fill(element.Message(), depth+1)
				list.Append(element)
			} else {
				list.Append(b.scalar(field, depth))
			}
		}
	case isMessageField(field):
		if depth < randomMessageMaxDepth {
			b.fill(m.Mutable(field).Message(), depth+1)
		}
	default:
		m.Set(field, b.scalar(field, depth))
	}
}

func isMessageField(field protoreflect.FieldDescriptor) bool {
	return field.Kind() == protoreflect.MessageKind || field.Kind() == protoreflect.GroupKind
}

func (b *randomMessageBuilder) scalar(field protoreflect.FieldDescriptor, depth int) protoreflect.Value {
	switch field.Kind() {
	case protoreflect.BoolKind:
		return protoreflect.ValueOfBool(b.rand.IntN(2) == 0)
	case protoreflect.EnumKind:
		values := field.Enum().Values()
		return protoreflect.ValueOfEnum(values.Get(b.rand.IntN(values.Len())).Number())
	case protoreflect.Int32Kind, protoreflect.Sint32Kind, protoreflect.Sfixed32Kind:
		return protoreflect.ValueOfInt32(b.rand.Int32N(1000))
	case protoreflect.Int64Kind, protoreflect.Sint64Kind, protoreflect.Sfixed64Kind:
		return protoreflect.ValueOfInt64(b.rand.Int64N(100000))
	case protoreflect.Uint32Kind, protoreflect.Fixed32Kind:
		return protoreflect.ValueOfUint32(b.rand.Uint32N(1000))
	case protoreflect.Uint64Kind, protoreflect.Fixed64Kind:
		return protoreflect.ValueOfUint64(b.rand.Uint64N(100000))
	case protoreflect.FloatKind:
		return protoreflect.ValueOfFloat32(b.rand.Float32())
	case protoreflect.DoubleKind:
		return protoreflect.ValueOfFloat64(b.rand.Float64())
	case protoreflect.StringKind:
		return protoreflect.ValueOfString(randomStrings[b.rand.IntN(len(randomStrings))])
	case protoreflect.BytesKind:
		switch b.rand.IntN(3) {
		case 0:
			// Translatable names as raw bytes. The visitors must leave them unchanged.
			return protoreflect.ValueOfBytes([]byte("orig AudienceId"))
		case 1:
			data := make([]byte, b.rand.IntN(64))
			for i := range data {
				data[i] = byte(b.rand.Uint32())
			}
			return protoreflect.ValueOfBytes(data)
		default:
			return protoreflect.ValueOfBytes(b.historyBytes(depth))
		}
	default:
		panic("unhandled field kind " + field.Kind().String())
	}
}

// historyBytes returns a serialized history of one to three random events.
func (b *randomMessageBuilder) historyBytes(depth int) []byte {
	if depth >= randomMessageMaxDepth {
		return nil
	}
	var events history.History
	for range 1 + b.rand.IntN(3) {
		var event history.HistoryEvent
		b.fill(event.ProtoReflect(), depth+1)
		events.Events = append(events.Events, &event)
	}
	data, err := proto.Marshal(&events)
	if err != nil {
		panic(err)
	}
	return data
}
