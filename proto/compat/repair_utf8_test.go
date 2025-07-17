package compat

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/failure/v1"
	"go.temporal.io/server/api/adminservice/v1"

	"github.com/temporalio/s2s-proxy/common"
	failure122 "github.com/temporalio/s2s-proxy/proto/1_22/api/failure/v1"
	adminservice122 "github.com/temporalio/s2s-proxy/proto/1_22/server/api/adminservice/v1"
	replication122 "github.com/temporalio/s2s-proxy/proto/1_22/server/api/replication/v1"
)

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
											Message: "abc\x2c",
											Cause: &failure122.Failure{
												Message: "abc\xc2",
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
			msg:       &failure122.Failure{Message: "abc\xc2"},
		},
		{
			name:      "nested invalid utf8",
			expChange: true,
			msg: &failure122.Failure{
				Message: "abc\xc2",
				Cause: &failure122.Failure{
					Message: "def\xc2",
				},
			},
		},
		{
			name:      "very nested invalid utf8",
			expChange: true,
			msg: &failure122.Failure{
				Message: "a\xc2",
				Cause: &failure122.Failure{
					Message: "b\xc2",
					Cause: &failure122.Failure{
						Message: "c\xc2",
						Cause: &failure122.Failure{
							Message: "d\xc2",
							Cause: &failure122.Failure{
								Message: "e\xc2",
								Cause: &failure122.Failure{
									Message: "f\xc2",
									Cause: &failure122.Failure{
										Message: "g\xc2",
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
								Message: "a\xc2",
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
