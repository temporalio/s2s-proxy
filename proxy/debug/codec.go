package debug

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"

	failure122 "github.com/temporalio/s2s-proxy/common/proto/1_22/api/failure/v1"
)

const (
	CodecName string = "s2s-proxy-debug-v2"
)

type CodecV2 struct {
	delegate encoding.CodecV2
}

func init() {
	encoding.RegisterCodecV2(&CodecV2{
		encoding.GetCodecV2(proto.Name),
	})
}

type unmarshaler interface { // TODO: rename
	Unmarshal([]byte) error
	Marshal() ([]byte, error)
}

// Marshal implements encoding.CodecV2.
func (c *CodecV2) Marshal(v any) (mem.BufferSlice, error) {
	out, err := c.delegate.Marshal(v)
	if err != nil && strings.Contains(err.Error(), "invalid UTF-8") {
		fmt.Printf("Marshal UTF-8 error: %s\n%v", err,
			base64.StdEncoding.EncodeToString(out.Materialize()),
		)
	}
	return out, err
}

// Name implements encoding.CodecV2.
func (c *CodecV2) Name() string {
	return "s2s-proxy-debug-v2"
}

// Unmarshal implements encoding.CodecV2.
func (c *CodecV2) Unmarshal(data mem.BufferSlice, v any) error {
	err := c.delegate.Unmarshal(data, v)
	if err != nil && strings.Contains(err.Error(), "invalid UTF-8") {
		fmt.Printf("Unmarshal UTF-8 error: v=%T %s\n%v\n", v, err,
			base64.StdEncoding.EncodeToString(data.Materialize()))

		vMarshaler, ok := v.(unmarshaler)
		if !ok {
			fmt.Printf("cannot convert any to unmarshaler: %T\n", v)
		}

		msg122, ok := frontendConvertTo122(v)
		if !ok {
			msg122, ok = adminConvertTo122(v)
			if !ok {
				fmt.Printf("could not convert to 122 for UTF-8 repair: %T\n", v)
				return err
			}
		}
		if msg122 == nil {
			return err
		}

		if err := msg122.Unmarshal(data.Materialize()); err != nil {
			fmt.Printf("failed to unmarshal 122 type: current=%T 122=%T\n", v, msg122)
			return err
		}

		changed := repairInvalidUTF8(msg122)
		if !changed {
			fmt.Printf("did not repair invalid UTF-8 in type v=%T v122=%T (no change)\n", v, msg122)
			return err
		}

		repaired, merr := msg122.Marshal()
		if merr != nil {
			fmt.Printf("failed to re-marshal message with repaired UTF-8: %v\n", err)
			return err // original error
		}
		fmt.Printf("Repaired UTF-8 msg: v=%T v122=%T\n%v\n", v, msg122,
			base64.StdEncoding.EncodeToString(data.Materialize()))

		merr = vMarshaler.Unmarshal(repaired)
		if merr != nil {
			fmt.Printf("failed to re-unmarshal repaired UTF-8: %v\n", err)
			return err
		}
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
