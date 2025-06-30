package debug

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unicode/utf8"

	"go.temporal.io/api/common/v1"
	"go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"go.uber.org/fx"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/encoding/proto"
	"google.golang.org/grpc/mem"

	adminservice122 "github.com/temporalio/s2s-proxy/common/proto/1_22/server/api/adminservice/v1"
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

// GetCodec returns our codec instance.
func GetCodec() *CodecV2 {
	return encoding.GetCodecV2(CodecName).(*CodecV2)
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
	c.Logger.Info("Unmarshal", tag.NewStringTag("type", fmt.Sprintf("%v", v)))
	err := c.delegate.Unmarshal(data, v)
	if err != nil && strings.Contains(err.Error(), "invalid UTF-8") {
		// TODO:
		// - Handle all relevant unary requests (workflowservice, adminservice).
		// - See if this is recursive (makes it easier - could just handle the specific types - string / blob)
		// - If not, probably codegen to be efficient
		//
		// - Try to handle history event / datablob here
		//
		// - Add metrics
		switch resp := v.(type) {
		case *common.DataBlob:
			// handle it
		case *adminservice.StreamWorkflowReplicationMessagesResponse:
			var resp122 adminservice122.StreamWorkflowReplicationMessagesResponse
			err := resp122.Unmarshal(data.Materialize())
			if err != nil {
				return err
			}

			changed, err := validateAndRepairReplicationTask(&resp122)
			if err != nil {
				c.Logger.Error("failed to repair invalid UTF-8",
					tag.NewErrorTag("error", err),
					tag.NewStringTag("type", fmt.Sprintf("%T", resp)),
				)
			} else if changed {
				repaired, err := resp122.Marshal()
				if err != nil {
					c.Logger.Error("failed to marshal repaired UTF-8",
						tag.NewErrorTag("error", err),
						tag.NewStringTag("type", fmt.Sprintf("%T", resp)),
					)
				} else {
					err := resp.Unmarshal(repaired)
					if err != nil {
						c.Logger.Error("failed to unmarshal repaired UTF-8",
							tag.NewErrorTag("error", err),
							tag.NewStringTag("type", fmt.Sprintf("%T", resp)),
						)
					} else {
						return nil
					}
				}
			}
		default:
			c.Logger.Error("Unhandled type with invalid UTF-8 error",
				tag.NewStringTag("type", fmt.Sprintf("%T", resp)),
				tag.NewStringTag("data", base64.StdEncoding.EncodeToString(data.Materialize())),
			)
		}
	}
	return err
}

// In old versions of Temporal, it was possible that certain history events could
// be written with invalid UTF-8.
func validateAndRepairReplicationTask(resp *adminservice122.StreamWorkflowReplicationMessagesResponse) (bool, error) {
	var changed bool
	for _, task := range resp.GetMessages().GetReplicationTasks() {
		if task == nil || task.Attributes == nil {
			continue
		}
		cause := task.GetSyncActivityTaskAttributes().
			GetLastFailure().
			GetCause()
		if !utf8.ValidString(cause.GetMessage()) {
			changed = true
			cause.Message = strings.ToValidUTF8(cause.Message, string(utf8.RuneError))

			// TODO: Pass logger through to here
			fmt.Println("repaired invalid utf-8 in history event in SyncActivityTaskAttributes")
		}
	}
	return changed, nil
}

var _ encoding.CodecV2 = (*CodecV2)(nil)
