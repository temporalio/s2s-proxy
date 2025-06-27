package interceptor

import (
	commonpb "go.temporal.io/api/common/v1"
	enumspb "go.temporal.io/api/enums/v1"
	historypb "go.temporal.io/api/history/v1"
	"go.temporal.io/server/common/persistence/serialization"
	"go.temporal.io/server/common/utf8validator"
	"google.golang.org/protobuf/proto"
)

type (
	// We need a serialization implementation that does not fail on invalid utf-8 strings.
	Serializer interface {
		SerializeEvents(batch []*historypb.HistoryEvent, encodingType enumspb.EncodingType) (*commonpb.DataBlob, error)
		DeserializeEvents(data *commonpb.DataBlob) ([]*historypb.HistoryEvent, error)
	}

	marshaler interface {
		Marshal() ([]byte, error)
	}

	// This is copied from temporal, only to disable the utf8 validation.
	//
	// TODO(pglass): I think latest version of the serializer in Temporal dropped the utf8 check.
	// So we may be able to upgrade the temporal version remove this custom serializer
	serializerImpl struct {
		enableUTF8Validation bool
	}
)

func NewSerializer() Serializer {
	return &serializerImpl{}
}

func (t *serializerImpl) SerializeEvents(events []*historypb.HistoryEvent, encodingType enumspb.EncodingType) (*commonpb.DataBlob, error) {
	return t.serialize(&historypb.History{Events: events}, encodingType)
}

func (t *serializerImpl) DeserializeEvents(data *commonpb.DataBlob) ([]*historypb.HistoryEvent, error) {
	if data == nil {
		return nil, nil
	}
	if len(data.Data) == 0 {
		return nil, nil
	}

	events := &historypb.History{}
	var err error
	switch data.EncodingType {
	case enumspb.ENCODING_TYPE_PROTO3:
		// Client API currently specifies encodingType on requests which span multiple of these objects
		err = events.Unmarshal(data.Data)
	default:
		return nil, serialization.NewUnknownEncodingTypeError(data.EncodingType.String(), enumspb.ENCODING_TYPE_PROTO3)
	}
	if t.enableUTF8Validation {
		if err == nil {
			err = utf8validator.Validate(events, utf8validator.SourcePersistence)
		}
	}
	if err != nil {
		return nil, serialization.NewDeserializationError(enumspb.ENCODING_TYPE_PROTO3, err)
	}
	return events.Events, nil
}

func (t *serializerImpl) serialize(p marshaler, encodingType enumspb.EncodingType) (*commonpb.DataBlob, error) {
	if p == nil {
		return nil, nil
	}

	var data []byte
	var err error

	switch encodingType {
	case enumspb.ENCODING_TYPE_PROTO3:
		if t.enableUTF8Validation {
			// Client API currently specifies encodingType on requests which span multiple of these objects
			if msg, ok := p.(proto.Message); ok {
				if err := utf8validator.Validate(msg, utf8validator.SourcePersistence); err != nil {
					return nil, serialization.NewSerializationError(enumspb.ENCODING_TYPE_PROTO3, err)
				}
			}
		}
		data, err = p.Marshal()
	default:
		return nil, serialization.NewUnknownEncodingTypeError(encodingType.String(), enumspb.ENCODING_TYPE_PROTO3)
	}

	if err != nil {
		return nil, serialization.NewSerializationError(enumspb.ENCODING_TYPE_PROTO3, err)
	}

	// Shouldn't happen, but keeping
	if data == nil {
		return nil, nil
	}

	return &commonpb.DataBlob{
		Data:         data,
		EncodingType: encodingType,
	}, nil
}
