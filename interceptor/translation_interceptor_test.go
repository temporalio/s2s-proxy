package interceptor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/temporalio/s2s-proxy/common"
)

// spyTranslator records how many times each Translator method is invoked
// so tests can verify whether translation logic ran.
type spyTranslator struct {
	matchCalls         int
	translateReqCalls  int
	translateRespCalls int
	// pairedReqs records the request passed alongside each response, in call order.
	pairedReqs []any
}

func (s *spyTranslator) Kind() string                       { return "spy" }
func (s *spyTranslator) MatchMethod(string) bool            { s.matchCalls++; return true }
func (s *spyTranslator) TranslateRequest(any) (bool, error) { s.translateReqCalls++; return false, nil }
func (s *spyTranslator) TranslateResponse(req, _ any) (bool, error) {
	s.translateRespCalls++
	s.pairedReqs = append(s.pairedReqs, req)
	return false, nil
}

func TestTranslationInterceptor(t *testing.T) {
	logger := log.NewTestLogger()
	handler := func(_ context.Context, _ any) (any, error) {
		return &workflowservice.DescribeWorkflowExecutionResponse{}, nil
	}

	cases := []struct {
		name string
		// fullMethod is the gRPC method the interceptor sees.
		fullMethod string
		// incomingHeaders is attached to the request context (nil for none).
		incomingHeaders map[string]string
		// MatchMethod is consulted once per phase (request + response) when translation runs.
		expectedMatchCalls int
		expectedReqCalls   int
		expectedRespCalls  int
	}{
		{
			name:               "header_false_skips_translators",
			fullMethod:         api.WorkflowServicePrefix + "DescribeWorkflowExecution",
			incomingHeaders:    map[string]string{common.RequestTranslationHeaderName: "false"},
			expectedMatchCalls: 0,
			expectedReqCalls:   0,
			expectedRespCalls:  0,
		},
		{
			name:               "workflow_method_invokes_translators",
			fullMethod:         api.WorkflowServicePrefix + "DescribeWorkflowExecution",
			expectedMatchCalls: 2,
			expectedReqCalls:   1,
			expectedRespCalls:  1,
		},
		{
			name:               "admin_method_invokes_translators",
			fullMethod:         api.AdminServicePrefix + "DescribeMutableState",
			expectedMatchCalls: 2,
			expectedReqCalls:   1,
			expectedRespCalls:  1,
		},
		{
			name:               "operator_method_invokes_translators",
			fullMethod:         api.OperatorServicePrefix + "DeleteNamespace",
			expectedMatchCalls: 2,
			expectedReqCalls:   1,
			expectedRespCalls:  1,
		},
		{
			name:               "other_service_skips_translators",
			fullMethod:         "/grpc.health.v1.Health/Check",
			expectedMatchCalls: 0,
			expectedReqCalls:   0,
			expectedRespCalls:  0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyTranslator{}
			ti := NewTranslationInterceptor(logger, []Translator{spy})
			info := &grpc.UnaryServerInfo{FullMethod: tc.fullMethod}

			ctx := context.Background()
			if tc.incomingHeaders != nil {
				ctx = metadata.NewIncomingContext(ctx, metadata.New(tc.incomingHeaders))
			}
			req := &workflowservice.DescribeWorkflowExecutionRequest{}
			_, err := ti.Intercept(ctx, req, info, handler)
			require.NoError(t, err)

			require.Equal(t, tc.expectedMatchCalls, spy.matchCalls, "MatchMethod call count")
			require.Equal(t, tc.expectedReqCalls, spy.translateReqCalls, "TranslateRequest call count")
			require.Equal(t, tc.expectedRespCalls, spy.translateRespCalls, "TranslateResponse call count")
			if tc.expectedRespCalls > 0 {
				require.Equal(t, []any{req}, spy.pairedReqs,
					"unary responses must be translated alongside their request")
			}
		})
	}
}

// fakeServerStream captures the messages a streamTranslator forwards.
type fakeServerStream struct {
	grpc.ServerStream
	sent []any
}

func (f *fakeServerStream) SendMsg(m any) error {
	f.sent = append(f.sent, m)
	return nil
}

func TestStreamTranslatorSendMsgHasNoPairedRequest(t *testing.T) {
	spy := &spyTranslator{}
	inner := &fakeServerStream{}
	st := newStreamTranslator(inner, log.NewTestLogger(), []Translator{spy})

	msg := &workflowservice.DescribeWorkflowExecutionResponse{}
	require.NoError(t, st.SendMsg(msg))

	require.Equal(t, 1, spy.translateRespCalls)
	require.Equal(t, []any{nil}, spy.pairedReqs, "stream messages have no request to pair with")
	require.Equal(t, []any{msg}, inner.sent)
}
