package compat

import (
	"fmt"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.uber.org/fx"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"

	"github.com/temporalio/s2s-proxy/common"
)

const (
	CodecName string = "s2s-proxy-codec"
)

type (
	// RepairUTF8Codec wraps the default proto codec. If the default codec
	// returns an invalid utf8 error, this will try to repair the utf8 string
	// automatically. We must use the codec to do this rather than an interceptor
	// because the invalid utf8 error will occur during deserialization, which
	// occurs before the interceptor is invoked.
	RepairUTF8Codec struct {
		delegate encoding.CodecV2
		*CodecParams
	}

	CodecParams struct {
		fx.In

		Logger log.Logger
	}
)

func init() {
	encoding.RegisterCodecV2(&RepairUTF8Codec{
		delegate: encoding.GetCodecV2(proto.Name), // Grab the default codec.
		CodecParams: &CodecParams{
			// Default to noop logger. This is replaced at startup with a configured logger.
			Logger: log.NewNoopLogger(),
		},
	})
}

// GetCodec returns our codec singleton.
func GetCodec() *RepairUTF8Codec {
	return encoding.GetCodecV2(CodecName).(*RepairUTF8Codec)
}

// Name implements encoding.CodecV2.
func (c *RepairUTF8Codec) Name() string {
	return CodecName
}

// Unmarshal implements encoding.CodecV2.
func (c *RepairUTF8Codec) Unmarshal(data mem.BufferSlice, v any) error {
	err := c.delegate.Unmarshal(data, v)
	if common.IsInvalidUTF8Error(err) {
		if err := convertAndRepairInvalidUTF8(data.Materialize(), v); err != nil {
			c.Logger.Error("during UTF-8 repair", tag.NewErrorTag("error", err))
		} else {
			c.Logger.Info("repaired invalid UTF-8 string", tag.NewStringTag("type", fmt.Sprintf("%T", v)))
			return nil
		}
	}
	return err
}

// Marshal implements encoding.CodecV2.
func (c *RepairUTF8Codec) Marshal(v any) (mem.BufferSlice, error) {
	out, err := c.delegate.Marshal(v)
	if common.IsInvalidUTF8Error(err) {
		// We have no known cases where marshalling would fail due to invalid UTF-8.
		c.Logger.Error("unhandled invalid UTF-8 error during marshalling", tag.NewErrorTag("error", err))
	}
	return out, err
}

var _ encoding.CodecV2 = (*RepairUTF8Codec)(nil)
