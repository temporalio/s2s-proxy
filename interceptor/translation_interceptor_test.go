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
}

func (s *spyTranslator) Kind() string                       { return "spy" }
func (s *spyTranslator) MatchMethod(string) bool            { s.matchCalls++; return true }
func (s *spyTranslator) TranslateRequest(any) (bool, error) { s.translateReqCalls++; return false, nil }
func (s *spyTranslator) TranslateResponse(any) (bool, error) {
	s.translateRespCalls++
	return false, nil
}

func TestTranslationInterceptor(t *testing.T) {
	logger := log.NewTestLogger()
	info := &grpc.UnaryServerInfo{
		FullMethod: api.WorkflowServicePrefix + "DescribeWorkflowExecution",
	}
	handler := func(_ context.Context, _ any) (any, error) {
		return &workflowservice.DescribeWorkflowExecutionResponse{}, nil
	}

	cases := []struct {
		name string
		// incomingHeaders is attached to the request context (nil for none).
		incomingHeaders map[string]string
		// MatchMethod is consulted once per phase (request + response) when translation runs.
		expectedMatchCalls int
		expectedReqCalls   int
		expectedRespCalls  int
	}{
		{
			name:               "header_false_skips_translators",
			incomingHeaders:    map[string]string{common.RequestTranslationHeaderName: "false"},
			expectedMatchCalls: 0,
			expectedReqCalls:   0,
			expectedRespCalls:  0,
		},
		{
			name:               "header_absent_invokes_translators",
			incomingHeaders:    nil,
			expectedMatchCalls: 2,
			expectedReqCalls:   1,
			expectedRespCalls:  1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spy := &spyTranslator{}
			ti := NewTranslationInterceptor(logger, []Translator{spy})

			ctx := context.Background()
			if tc.incomingHeaders != nil {
				ctx = metadata.NewIncomingContext(ctx, metadata.New(tc.incomingHeaders))
			}
			_, err := ti.Intercept(ctx, &workflowservice.DescribeWorkflowExecutionRequest{}, info, handler)
			require.NoError(t, err)

			require.Equal(t, tc.expectedMatchCalls, spy.matchCalls, "MatchMethod call count")
			require.Equal(t, tc.expectedReqCalls, spy.translateReqCalls, "TranslateRequest call count")
			require.Equal(t, tc.expectedRespCalls, spy.translateRespCalls, "TranslateResponse call count")
		})
	}
}
