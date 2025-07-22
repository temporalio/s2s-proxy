package compat

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/server/api/adminservice/v1"
	replication "go.temporal.io/server/api/replication/v1"

	"github.com/temporalio/s2s-proxy/common"
	failure122 "github.com/temporalio/s2s-proxy/proto/1_22/api/failure/v1"
	adminservice122 "github.com/temporalio/s2s-proxy/proto/1_22/server/api/adminservice/v1"
	replication122 "github.com/temporalio/s2s-proxy/proto/1_22/server/api/replication/v1"
)

func TestConvertAndRepairInvalidUTF8(t *testing.T) {
	cases := []struct {
		name     string
		expError string
		getType  func() any
		getData  func() []byte
	}{
		{
			name:     "error - type is not a marshaler",
			expError: "could not cast struct {} as a marshaler",
			getType: func() any {
				return struct{}{}
			},
		},
		{
			name:     "error - cannot convert to gogo type",
			expError: "could not convert *repication.ReplicationTask to gogo-based protobuf type",
			getType: func() any {
				// Only the top-level request/response types are supported by convertAndRepairInvalidUTF8
				return &replication.ReplicationTask{}
			},
		},
		{
			name:     "error - nothing was changed",
			expError: "nothing was repaired in type *adminservice.StreamWorkflowReplicationMessagesResponse",
			getType: func() any {
				return &adminservice.StreamWorkflowReplicationMessagesResponse{}
			},
			getData: func() []byte {
				msg := makeStreamWorkflowReplicationMessages("abc")
				data, err := msg.Marshal()
				require.NoError(t, err)
				return data
			},
		},
		{
			name: "StreamWorkflowReplicationMessagesResponse with invalid utf8",
			getType: func() any {
				return &adminservice.StreamWorkflowReplicationMessagesResponse{}
			},
			getData: func() []byte {
				// Make a message, and then replace "abcXX" with invalid utf8.
				// (Make sure not to change the byte length when replacing)
				msg := makeStreamWorkflowReplicationMessages("abcXX")
				data, err := msg.Marshal()
				require.NoError(t, err)

				data = bytes.ReplaceAll(data, []byte("abcXX"), []byte("asd\xff\xff"))

				// make sure we have invalid utf8 data
				require.ErrorContains(t, msg.Unmarshal(data), "contains invalid UTF-8")
				return data
			},
		},
	}

	for _, tc := range cases {
		if tc.getData == nil {
			tc.getData = func() []byte { return nil }
		}
		t.Run(tc.name, func(t *testing.T) {
			msg := tc.getType()
			err := convertAndRepairInvalidUTF8(tc.getData(), msg)
			if len(tc.expError) != 0 {
				require.ErrorContains(t, err, tc.expError)
			} else {
				require.NoError(t, err)
			}

			if err == nil {
				checkMarshalUnmarshal(t, msg.(common.Marshaler), tc.getType().(common.Marshaler))
			}
		})
	}
}

func TestRepairUTF8(t *testing.T) {
	cases := []struct {
		name                string
		expChange           bool
		expError            string
		mustDeserializeInto common.Marshaler
		msg122              common.Marshaler
	}{
		{

			name:   "nil",
			msg122: nil,
		},
		{
			name:                "Failure in SyncActivityTaskAttributes in StreamWorkflowReplicationMessages",
			expChange:           true,
			mustDeserializeInto: &adminservice.StreamWorkflowReplicationMessagesResponse{},
			msg122: &adminservice122.StreamWorkflowReplicationMessagesResponse{
				Attributes: &adminservice122.StreamWorkflowReplicationMessagesResponse_Messages{
					Messages: &replication122.WorkflowReplicationMessages{
						ReplicationTasks: []*replication122.ReplicationTask{
							{
								TaskType:     1,
								SourceTaskId: 2,
								Attributes: &replication122.ReplicationTask_SyncActivityTaskAttributes{
									SyncActivityTaskAttributes: &replication122.SyncActivityTaskAttributes{
										NamespaceId: "ns-id",
										WorkflowId:  "wf-id",
										RunId:       "run-id",
										LastFailure: &failure122.Failure{
											Message: "abc\xff\xff",
											Cause: &failure122.Failure{
												Message: "abc\xff\xff",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed, err := repairInvalidUTF8(tc.msg122)
			require.Equal(t, tc.expChange, changed)
			if len(tc.expError) != 0 {
				require.ErrorContains(t, err, tc.expError)
			} else {
				require.NoError(t, err)
			}

			if tc.msg122 != nil && tc.mustDeserializeInto != nil {
				checkMarshalUnmarshal(t, tc.msg122, tc.mustDeserializeInto)
			}
		})
	}

}

func TestRepairUTF8InFailure(t *testing.T) {
	cases := []struct {
		name      string
		expChange bool
		expError  string
		msg       *failure122.Failure
	}{
		{
			name: "nil",
			msg:  nil,
		},
		{
			name: "empty",
			msg:  &failure122.Failure{},
		},
		{
			name: "basic valid",
			msg:  &failure122.Failure{Message: "abc"},
		},
		{
			name:      "basic invalid utf8",
			expChange: true,
			msg:       &failure122.Failure{Message: "abc\xff\xff"},
		},
		{
			name:      "nested invalid utf8",
			expChange: true,
			msg: &failure122.Failure{
				Message: "abc\xff\xff",
				Cause: &failure122.Failure{
					Message: "def\xff\xff",
				},
			},
		},
		{
			name:      "very nested invalid utf8",
			expChange: true,
			msg: &failure122.Failure{
				Message: "a\xff\xff",
				Cause: &failure122.Failure{
					Message: "b\xff\xff",
					Cause: &failure122.Failure{
						Message: "c\xff\xff",
						Cause: &failure122.Failure{
							Message: "d\xff\xff",
							Cause: &failure122.Failure{
								Message: "e\xff\xff",
								Cause: &failure122.Failure{
									Message: "f\xff\xff",
									Cause: &failure122.Failure{
										Message: "g\xff\xff",
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:      "max recursion depth",
			expChange: true,
			expError:  "reached maximum failure chain depth",
			msg: &failure122.Failure{
				Cause: &failure122.Failure{
					Cause: &failure122.Failure{
						Cause: &failure122.Failure{
							Cause: &failure122.Failure{
								Message: "a\xff\xff",
								Cause: &failure122.Failure{
									Cause: &failure122.Failure{
										Cause: &failure122.Failure{
											Cause: &failure122.Failure{
												Cause: &failure122.Failure{
													Cause: &failure122.Failure{
														Cause: &failure122.Failure{
															Cause: &failure122.Failure{},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed, err := repairInvalidUTF8InFailure(tc.msg)
			require.Equal(t, tc.expChange, changed)
			if len(tc.expError) != 0 {
				require.ErrorContains(t, err, tc.expError)
			} else {
				require.NoError(t, err)
			}

			// check that we can deserialize with new protos
			// this ensures that the invalid utf-8 was really fixed
			if tc.msg != nil {
				var f failure.Failure
				checkMarshalUnmarshal(t, tc.msg, &f)
			}
		})
	}
}

func checkMarshalUnmarshal(t *testing.T, from, to common.Marshaler) {
	data, err := from.Marshal()
	require.NoError(t, err, "failed to marshal message")
	require.NoError(t, to.Unmarshal(data), "failed to unmarshal message")
}

func makeStreamWorkflowReplicationMessages(failureMessage string) *adminservice.StreamWorkflowReplicationMessagesResponse {
	return &adminservice.StreamWorkflowReplicationMessagesResponse{
		Attributes: &adminservice.StreamWorkflowReplicationMessagesResponse_Messages{
			Messages: &replication.WorkflowReplicationMessages{
				ReplicationTasks: []*replication.ReplicationTask{
					{
						TaskType:     1,
						SourceTaskId: 2,
						Attributes: &replication.ReplicationTask_SyncActivityTaskAttributes{
							SyncActivityTaskAttributes: &replication.SyncActivityTaskAttributes{
								NamespaceId: "ns-id",
								WorkflowId:  "wf-id",
								RunId:       "run-id",
								LastFailure: &failure.Failure{
									// We'll serialize and then replace this with invalid utf-8.
									Message: failureMessage,
									Cause: &failure.Failure{
										Message: failureMessage,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}
