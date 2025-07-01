package debug

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.uber.org/fx"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"

	failure122 "github.com/temporalio/s2s-proxy/common/proto/1_22/api/failure/v1"
)

const (
	CodecName string = "s2s-proxy-debug-v2"
)

type (
	CodecV2 struct {
		delegate encoding.CodecV2
		*Params
	}

	Params struct {
		fx.In

		Logger log.Logger
	}
)

func init() {
	encoding.RegisterCodecV2(&CodecV2{
		delegate: encoding.GetCodecV2(proto.Name),
		Params: &Params{
			// Default to noop logger to avoid nil pointer.
			// This is replaced at startup with a configured logger.
			Logger: log.NewNoopLogger(),
		},
	})
}

type unmarshaler interface { // TODO: rename
	Unmarshal([]byte) error
	Marshal() ([]byte, error)
}

// GetCodec returns our codec instance.
func GetCodec() *CodecV2 {
	return encoding.GetCodecV2(CodecName).(*CodecV2)
}

// Marshal implements encoding.CodecV2.
func (c *CodecV2) Marshal(v any) (mem.BufferSlice, error) {
	out, err := c.delegate.Marshal(v)
	if err != nil && strings.Contains(err.Error(), "invalid UTF-8") {
		c.Logger.Warn("Marshal UTF-8 error", tag.NewErrorTag("error", err))
	}
	return out, err
}

// Name implements encoding.CodecV2.
func (c *CodecV2) Name() string {
	return "s2s-proxy-debug-v2"
}

// Unmarshal implements encoding.CodecV2.
func (c *CodecV2) Unmarshal(data mem.BufferSlice, v any) error {
	c.Logger.Info("Unmarshal", tag.NewStringTag("type", fmt.Sprintf("%v", v)))
	err := c.delegate.Unmarshal(data, v)
	if err != nil && strings.Contains(err.Error(), "invalid UTF-8") {
		c.Logger.Warn("invalid UTF-8 error encountered during unmarshal; attempting to repair")

		vMarshaler, ok := v.(unmarshaler)
		if !ok {
			c.Logger.Error("during UTF-8 repair, could not cast as an unmarshaler")
			return err
		}

		msg122, ok := frontendConvertTo122(v)
		if !ok {
			msg122, ok = adminConvertTo122(v)
			if !ok {
				c.Logger.Error("during UTF-8 repair, could not convert to gogo-based protobuf type",
					tag.NewStringTag("type", fmt.Sprintf("%T", v)),
				)
				return err
			}
		}
		if msg122 == nil {
			return err
		}

		if err := msg122.Unmarshal(data.Materialize()); err != nil {
			c.Logger.Error("during UTF-8 repair, could not unmarshal as gogo-based protobuf type",
				tag.NewErrorTag("error", err),
				tag.NewStringTag("type", fmt.Sprintf("%T", v)),
			)
			return err
		}

		changed := repairInvalidUTF8(msg122)
		if !changed {
			c.Logger.Error("during UTF-8 repair, nothing was repaired",
				tag.NewStringTag("type", fmt.Sprintf("%T", v)),
			)
			return err
		}

		if repaired, err := msg122.Marshal(); err != nil {
			c.Logger.Error("during UTF-8 repair, failed to re-marshal message",
				tag.NewErrorTag("error", err),
				tag.NewStringTag("type", fmt.Sprintf("%T", msg122)),
			)
			return err
		} else if err := vMarshaler.Unmarshal(repaired); err != nil {
			c.Logger.Error("during UTF-8 repair, failed to re-unmarshal message",
				tag.NewErrorTag("error", err),
				tag.NewStringTag("type", fmt.Sprintf("%T", msg122)),
			)
			return err
		}

		c.Logger.Info("repaired invalid UTF-8",
			tag.NewStringTag("type", fmt.Sprintf("%T", msg122)),
		)
		return nil
	}
	return err
}

// In old versions of Temporal, it was possible that certain history events could
// be written with invalid UTF-8.
func repairUTF8InLastFailure(lastFailure *failure122.Failure) bool {
	cause := lastFailure.GetCause()
	if !utf8.ValidString(cause.GetMessage()) {
		cause.Message = strings.ToValidUTF8(cause.Message, string(utf8.RuneError))
		fmt.Println("repaired invalid utf-8 in last failure message")
		return true
	}
	return false
}

var _ encoding.CodecV2 = (*CodecV2)(nil)
