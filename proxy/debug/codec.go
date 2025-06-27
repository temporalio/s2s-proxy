package debug

import (
	"encoding/base64"
	"fmt"
	"strings"

	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"
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
		fmt.Printf("Unmarshal UTF-8 error: v=%T %s\n%v", v, err,
			base64.StdEncoding.EncodeToString(data.Materialize()))
	}
	return err
}

var _ encoding.CodecV2 = (*CodecV2)(nil)
