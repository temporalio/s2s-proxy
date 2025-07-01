package debug

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.temporal.io/server/api/adminservice/v1"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"

	failure122 "github.com/temporalio/s2s-proxy/common/proto/1_22/api/failure/v1"
	adminservice122 "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/adminservice/v1"
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
		switch resp := v.(type) {
		case *adminservice.StreamWorkflowReplicationMessagesResponse:
			var resp122 adminservice122.StreamWorkflowReplicationMessagesResponse
			err := resp122.Unmarshal(data.Materialize())
			if err != nil {
				return err
			}

			if repairInvalidUTF8(&resp122) {
				repaired, err := resp122.Marshal()
				if err != nil {
					fmt.Printf("failed to re-marshal repair invalid utf8: %v\n", err)
					// return the original error?
				} else {
					err := resp.Unmarshal(repaired)
					if err != nil {
						fmt.Printf("failed to re-unmarshal repair invalid utf8: %v\n", err)
						// return the original error?
					} else {
						return nil
					}
				}
			}
		}
	}
	return err
}

// In old versions of Temporal, it was possible that certain history events could
// be written with invalid UTF-8.
func repairUTF8InLastFailure(lastFailure *failure122.Failure) bool {
	cause := lastFailure.GetCause()
	if !utf8.ValidString(cause.GetMessage()) {
		cause.Message = strings.ToValidUTF8(cause.Message, string(utf8.RuneError))
		fmt.Println("repaired invalid utf-8 in history event in SyncActivityTaskAttributes")
		return true
	}
	return false
}

var _ encoding.CodecV2 = (*CodecV2)(nil)
