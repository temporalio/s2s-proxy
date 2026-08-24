package interceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/common/v1"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
)

const (
	// proxyEndpoint is what the proxy writes into the payload.
	proxyEndpoint = "my-s2s-proxy.svc.cluster.local:9233"
	// cloudEndpoint is what the control plane sends and the local server cannot reach.
	cloudEndpoint = "admin.example-cell.cluster.tmprl.cloud:7233"

	// forceReplicationParamsJSON is the shape the control plane sends: the SDK's default json/plain
	// encoding of migration.ForceReplicationParams, whose fields carry no json tags.
	forceReplicationParamsJSON = `{
		"Namespace": "my-namespace",
		"Query": "",
		"ConcurrentActivityCount": 4,
		"OverallRps": 10,
		"EnableVerification": true,
		"TargetClusterEndpoint": "` + cloudEndpoint + `",
		"TargetClusterName": "example-cell"
	}`
	// rewrittenParamsJSON is forceReplicationParamsJSON with only TargetClusterEndpoint replaced.
	rewrittenParamsJSON = `{
		"Namespace": "my-namespace",
		"Query": "",
		"ConcurrentActivityCount": 4,
		"OverallRps": 10,
		"EnableVerification": true,
		"TargetClusterEndpoint": "` + proxyEndpoint + `",
		"TargetClusterName": "example-cell"
	}`
)

func payloadsWith(encoding string, data string) *common.Payloads {
	return &common.Payloads{
		Payloads: []*common.Payload{
			{
				Metadata: map[string][]byte{
					"encoding": []byte(encoding),
					// A key we do not own, to prove Metadata survives the rewrite.
					"someOtherKey": []byte("someOtherValue"),
				},
				Data: []byte(data),
			},
		},
	}
}

func startWorkflowReq(workflowType string, input *common.Payloads) *workflowservice.StartWorkflowExecutionRequest {
	return &workflowservice.StartWorkflowExecutionRequest{
		Namespace:    "my-namespace",
		WorkflowId:   "force-replication-my-namespace",
		WorkflowType: &common.WorkflowType{Name: workflowType},
		Input:        input,
	}
}

func forceReplicationReq() *workflowservice.StartWorkflowExecutionRequest {
	return startWorkflowReq(forceReplicationWorkflowType, payloadsWith(jsonPlainEncoding, forceReplicationParamsJSON))
}

// payloadData returns the bytes of the first input payload, or nil if there is none.
func payloadData(req any) []byte {
	r, ok := req.(*workflowservice.StartWorkflowExecutionRequest)
	if !ok {
		return nil
	}
	payloads := r.GetInput().GetPayloads()
	if len(payloads) == 0 {
		return nil
	}
	return payloads[0].GetData()
}

func TestForceReplicationEndpointTranslator_TranslateRequest(t *testing.T) {
	cases := []struct {
		name string
		// req is a func so every case gets a fresh fixture.
		req         func() any
		wantChanged bool
		wantErr     bool
		// wantJSON, when set, is compared to the payload data after translation. When it is empty
		// the payload must come out byte-for-byte unchanged.
		wantJSON string
	}{
		{
			name:        "rewrites force replication endpoint",
			req:         func() any { return forceReplicationReq() },
			wantChanged: true,
			wantJSON:    rewrittenParamsJSON,
		},
		{
			name: "ignores other workflow types",
			req: func() any {
				return startWorkflowReq("some-other-workflow", payloadsWith(jsonPlainEncoding, forceReplicationParamsJSON))
			},
		},
		{
			name: "ignores other request types",
			req: func() any {
				return &workflowservice.DescribeWorkflowExecutionRequest{Namespace: "my-namespace"}
			},
		},
		{
			name:    "ignores untyped nil",
			req:     func() any { return nil },
			wantErr: false,
		},
		{
			name: "ignores typed nil request",
			req: func() any {
				var r *workflowservice.StartWorkflowExecutionRequest
				return r
			},
		},
		{
			name: "errors on nil input",
			req: func() any {
				return startWorkflowReq(forceReplicationWorkflowType, nil)
			},
			wantErr: true,
		},
		{
			name: "errors on empty payloads",
			req: func() any {
				return startWorkflowReq(forceReplicationWorkflowType, &common.Payloads{})
			},
			wantErr: true,
		},
		{
			name: "errors on nil payload",
			req: func() any {
				return startWorkflowReq(forceReplicationWorkflowType, &common.Payloads{Payloads: []*common.Payload{nil}})
			},
			wantErr: true,
		},
		{
			name: "errors on non-json encoding",
			req: func() any {
				return startWorkflowReq(forceReplicationWorkflowType, payloadsWith("binary/encrypted", "not json at all"))
			},
			wantErr: true,
		},
		{
			name: "errors on unparseable json",
			req: func() any {
				return startWorkflowReq(forceReplicationWorkflowType, payloadsWith(jsonPlainEncoding, "{not json"))
			},
			wantErr: true,
		},
		{
			name: "errors when the endpoint field is absent",
			req: func() any {
				return startWorkflowReq(forceReplicationWorkflowType, payloadsWith(jsonPlainEncoding, `{"TargetClusterName":"example-cell"}`))
			},
			wantErr: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := NewForceReplicationEndpointTranslator(log.NewTestLogger(), proxyEndpoint)
			req := c.req()
			before := append([]byte(nil), payloadData(req)...)

			changed, err := tr.TranslateRequest(req)

			if c.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, c.wantChanged, changed)

			if c.wantJSON != "" {
				require.JSONEq(t, c.wantJSON, string(payloadData(req)))
			} else {
				// Nothing was rewritten, including on the error paths.
				require.Equal(t, before, append([]byte(nil), payloadData(req)...))
			}
		})
	}
}

func TestForceReplicationEndpointTranslator_PreservesPayloadMetadata(t *testing.T) {
	tr := NewForceReplicationEndpointTranslator(log.NewTestLogger(), proxyEndpoint)
	req := forceReplicationReq()

	changed, err := tr.TranslateRequest(req)
	require.NoError(t, err)
	require.True(t, changed)

	require.Equal(t, map[string][]byte{
		"encoding":     []byte(jsonPlainEncoding),
		"someOtherKey": []byte("someOtherValue"),
	}, req.GetInput().GetPayloads()[0].GetMetadata())
	// Fields other than the endpoint, TargetClusterName in particular, are left alone.
	require.JSONEq(t, rewrittenParamsJSON, string(payloadData(req)))
}

func TestForceReplicationEndpointTranslator_Idempotent(t *testing.T) {
	tr := NewForceReplicationEndpointTranslator(log.NewTestLogger(), proxyEndpoint)
	req := forceReplicationReq()

	changed, err := tr.TranslateRequest(req)
	require.NoError(t, err)
	require.True(t, changed)
	once := append([]byte(nil), payloadData(req)...)

	// A second pass (retry, or a second proxy hop) must land on the same value.
	changed, err = tr.TranslateRequest(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, once, payloadData(req))
	require.JSONEq(t, rewrittenParamsJSON, string(payloadData(req)))
}

func TestForceReplicationEndpointTranslator_MatchMethod(t *testing.T) {
	// Pin the wire format so the cases below cannot pass vacuously.
	require.Equal(t, "/temporal.api.workflowservice.v1.WorkflowService/StartWorkflowExecution", startWorkflowExecutionMethod)

	cases := []struct {
		method    string
		wantMatch bool
	}{
		{method: api.WorkflowServicePrefix + "StartWorkflowExecution", wantMatch: true},
		{method: api.WorkflowServicePrefix + "DescribeWorkflowExecution"},
		{method: api.WorkflowServicePrefix + "SignalWithStartWorkflowExecution"},
		// WorkflowServicePrefix already ends in "/", so a second one is not the real method name.
		{method: api.WorkflowServicePrefix + "/StartWorkflowExecution"},
		{method: api.AdminServicePrefix + "StartWorkflowExecution"},
		{method: "StartWorkflowExecution"},
		{method: ""},
	}

	tr := NewForceReplicationEndpointTranslator(log.NewTestLogger(), proxyEndpoint)
	for _, c := range cases {
		t.Run(c.method, func(t *testing.T) {
			require.Equal(t, c.wantMatch, tr.MatchMethod(c.method))
		})
	}
}

func TestForceReplicationEndpointTranslator_TranslateResponse(t *testing.T) {
	tr := NewForceReplicationEndpointTranslator(log.NewTestLogger(), proxyEndpoint)

	for _, resp := range []any{
		nil,
		&workflowservice.StartWorkflowExecutionResponse{RunId: "run-id"},
		forceReplicationReq(),
	} {
		changed, err := tr.TranslateResponse(resp)
		require.NoError(t, err)
		require.False(t, changed)
	}
}

// TestForceReplicationEndpointTranslator_ViaInterceptor proves the translator is actually reached
// through the unary interceptor for the matched method, and only for that method.
func TestForceReplicationEndpointTranslator_ViaInterceptor(t *testing.T) {
	cases := []struct {
		name       string
		fullMethod string
		wantJSON   string
	}{
		{
			name:       "matched method is rewritten",
			fullMethod: api.WorkflowServicePrefix + "StartWorkflowExecution",
			wantJSON:   rewrittenParamsJSON,
		},
		{
			name:       "unmatched method is untouched",
			fullMethod: api.WorkflowServicePrefix + "SignalWithStartWorkflowExecution",
			wantJSON:   forceReplicationParamsJSON,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tr := NewForceReplicationEndpointTranslator(log.NewTestLogger(), proxyEndpoint)
			i := NewTranslationInterceptor(log.NewTestLogger(), []Translator{tr})

			var seenByHandler []byte
			handler := func(_ context.Context, req any) (any, error) {
				seenByHandler = append([]byte(nil), payloadData(req)...)
				return &workflowservice.StartWorkflowExecutionResponse{}, nil
			}

			_, err := i.Intercept(context.Background(), forceReplicationReq(),
				&grpc.UnaryServerInfo{FullMethod: c.fullMethod}, handler)
			require.NoError(t, err)
			require.JSONEq(t, c.wantJSON, string(seenByHandler))
		})
	}
}
