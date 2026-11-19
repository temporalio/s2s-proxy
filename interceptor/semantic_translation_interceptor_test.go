package interceptor

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	workflowservicepb "go.temporal.io/api/workflowservice/v1"
	adminservicepb "go.temporal.io/server/api/adminservice/v1"
	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/failoverversion"
	"github.com/temporalio/s2s-proxy/metrics"
)

type semanticSpy struct {
	name       string
	calls      *[]string
	messageErr error
	errorFn    func(error) error
}

func TestSemanticStreamReceiveBeforeTranslateAndSendFailsClosed(t *testing.T) {
	t.Parallel()
	calls := []string{}
	translationErr := failoverversion.NewTranslationError(
		failoverversion.ErrorUnknownLane, failoverversion.LocalToRemote, "message", "version", errors.New("invalid lane"),
	)
	interceptor := NewSemanticTranslationInterceptor(log.NewTestLogger(), []SemanticTranslationPair{{
		Requests:  &semanticSpy{name: "recv", calls: &calls},
		Responses: &semanticSpy{name: "send", calls: &calls, messageErr: translationErr},
	}})
	underlying := &semanticTestServerStream{recvErr: io.EOF}
	stream := &semanticStreamTranslator{ServerStream: underlying, interceptor: interceptor}

	err := stream.RecvMsg(&workflowservicepb.DescribeNamespaceRequest{})
	require.ErrorIs(t, err, io.EOF)
	require.Empty(t, calls, "translation must not run after an underlying receive error")

	err = stream.SendMsg(&workflowservicepb.DescribeNamespaceResponse{})
	require.Equal(t, codes.Internal, status.Code(err))
	require.False(t, underlying.sent, "failed translation must not reach the underlying stream")
}

type semanticTestServerStream struct {
	grpc.ServerStream
	recvErr error
	sent    bool
}

func (s *semanticTestServerStream) Context() context.Context { return context.Background() }
func (s *semanticTestServerStream) RecvMsg(any) error        { return s.recvErr }
func (s *semanticTestServerStream) SendMsg(any) error {
	s.sent = true
	return nil
}

type semanticCaptureServerStream struct {
	grpc.ServerStream
	received proto.Message
	sent     proto.Message
}

func (s *semanticCaptureServerStream) Context() context.Context { return context.Background() }
func (s *semanticCaptureServerStream) RecvMsg(message any) error {
	target := message.(proto.Message)
	proto.Reset(target)
	proto.Merge(target, s.received)
	return nil
}
func (s *semanticCaptureServerStream) SendMsg(message any) error {
	s.sent = proto.Clone(message.(proto.Message))
	return nil
}

func (s *semanticSpy) Kind() string { return "semantic-spy" }
func (s *semanticSpy) TranslateMessage(message proto.Message) (TranslationResult, error) {
	*s.calls = append(*s.calls, s.name)
	if s.messageErr != nil {
		return TranslationResult{}, s.messageErr
	}
	return TranslationResult{Message: proto.Clone(message), Report: TranslationReport{Changed: true}}, nil
}
func (s *semanticSpy) TranslateError(err error) ErrorTranslationResult {
	*s.calls = append(*s.calls, s.name+"-error")
	if s.errorFn != nil {
		return ErrorTranslationResult{Error: s.errorFn(err), Report: TranslationReport{Changed: true, StatusDetails: 1}}
	}
	return ErrorTranslationResult{Error: err}
}

func TestSemanticTranslationInterceptorOrder(t *testing.T) {
	t.Parallel()
	calls := []string{}
	interceptor := NewSemanticTranslationInterceptor(log.NewTestLogger(), []SemanticTranslationPair{
		{Requests: &semanticSpy{name: "request-1", calls: &calls}, Responses: &semanticSpy{name: "response-1", calls: &calls}},
		{Requests: &semanticSpy{name: "request-2", calls: &calls}, Responses: &semanticSpy{name: "response-2", calls: &calls}},
	})
	response, err := interceptor.Intercept(
		context.Background(),
		&workflowservicepb.DescribeNamespaceRequest{},
		&grpc.UnaryServerInfo{FullMethod: api.WorkflowServicePrefix + "DescribeNamespace"},
		func(context.Context, any) (any, error) {
			calls = append(calls, "handler")
			return &workflowservicepb.DescribeNamespaceResponse{}, nil
		},
	)
	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, []string{"request-1", "request-2", "handler", "response-2", "response-1"}, calls)
}

func TestSemanticTranslationInterceptorFailsClosed(t *testing.T) {
	t.Parallel()
	calls := []string{}
	translationErr := failoverversion.NewTranslationError(
		failoverversion.ErrorUnknownLane,
		failoverversion.LocalToRemote,
		"request",
		"version",
		errors.New("unsafe raw value 12345"),
	)
	interceptor := NewSemanticTranslationInterceptor(log.NewTestLogger(), []SemanticTranslationPair{{
		Requests:  &semanticSpy{name: "request", calls: &calls, messageErr: translationErr},
		Responses: &semanticSpy{name: "response", calls: &calls},
	}})
	handlerCalled := false
	_, err := interceptor.Intercept(
		context.Background(),
		&workflowservicepb.DescribeNamespaceRequest{},
		&grpc.UnaryServerInfo{FullMethod: api.WorkflowServicePrefix + "DescribeNamespace"},
		func(context.Context, any) (any, error) {
			handlerCalled = true
			return nil, nil
		},
	)
	require.False(t, handlerCalled)
	require.Equal(t, codes.Internal, status.Code(err))
	require.NotContains(t, err.Error(), "12345")
}

func TestSemanticTranslationInterceptorRawBypass(t *testing.T) {
	t.Parallel()
	calls := []string{}
	interceptor := NewSemanticTranslationInterceptor(log.NewTestLogger(), []SemanticTranslationPair{{
		Requests:  &semanticSpy{name: "request", calls: &calls},
		Responses: &semanticSpy{name: "response", calls: &calls},
	}})
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(common.RequestTranslationHeaderName, "false"))
	_, err := interceptor.Intercept(
		ctx,
		&workflowservicepb.DescribeNamespaceRequest{},
		&grpc.UnaryServerInfo{FullMethod: api.WorkflowServicePrefix + "DescribeNamespace"},
		func(context.Context, any) (any, error) { return &workflowservicepb.DescribeNamespaceResponse{}, nil },
	)
	require.NoError(t, err)
	require.Empty(t, calls)
}

func TestSemanticTranslationInterceptorPreservesTranslatedHandlerStatus(t *testing.T) {
	t.Parallel()
	calls := []string{}
	interceptor := NewSemanticTranslationInterceptor(log.NewTestLogger(), []SemanticTranslationPair{{
		Requests: &semanticSpy{name: "request", calls: &calls},
		Responses: &semanticSpy{name: "response", calls: &calls, errorFn: func(error) error {
			return status.Error(codes.Aborted, "translated detail")
		}},
	}})
	_, err := interceptor.Intercept(
		context.Background(),
		&workflowservicepb.DescribeNamespaceRequest{},
		&grpc.UnaryServerInfo{FullMethod: api.WorkflowServicePrefix + "DescribeNamespace"},
		func(context.Context, any) (any, error) { return nil, status.Error(codes.Aborted, "original") },
	)
	require.Equal(t, codes.Aborted, status.Code(err))
	require.Contains(t, err.Error(), "translated detail")
}

func TestSemanticTranslationInterceptorObservesAndCommitsClones(t *testing.T) {
	forward, inverse := testFailoverTranslator(t)
	observer := NewTranslationDebugObserver(2)
	interceptor := NewSemanticTranslationInterceptor(
		log.NewTestLogger(),
		[]SemanticTranslationPair{{Requests: forward, Responses: inverse}},
		WithSemanticTranslationObserver(observer),
	)
	original := &adminservicepb.DescribeClusterResponse{FailoverVersionIncrement: 105, InitialFailoverVersion: 1}
	requestOperationsBefore := testutil.ToFloat64(metrics.FailoverVersionTranslationOperations.WithLabelValues("local_to_remote", "success", "none"))
	responseOperationsBefore := testutil.ToFloat64(metrics.FailoverVersionTranslationOperations.WithLabelValues("remote_to_local", "success", "none"))
	response, err := interceptor.Intercept(
		context.Background(),
		original,
		&grpc.UnaryServerInfo{FullMethod: api.AdminServicePrefix + "DescribeCluster"},
		func(_ context.Context, request any) (any, error) {
			translated := request.(*adminservicepb.DescribeClusterResponse)
			require.Equal(t, int64(100), translated.GetFailoverVersionIncrement())
			require.NotSame(t, original, translated)
			return &adminservicepb.DescribeClusterResponse{FailoverVersionIncrement: 100, InitialFailoverVersion: 1}, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, int64(105), original.GetFailoverVersionIncrement(), "interceptor must not mutate caller input")
	require.Equal(t, int64(105), response.(*adminservicepb.DescribeClusterResponse).GetFailoverVersionIncrement())

	latest, lastSuccess, failures := observer.Snapshot()
	require.NotNil(t, latest)
	require.NotNil(t, lastSuccess)
	require.Equal(t, "success", latest.Outcome)
	require.Equal(t, failoverversion.RemoteToLocal, latest.Direction)
	require.Regexp(t, `^sha256:[0-9a-f]{64}$`, latest.MappingFingerprint)
	require.True(t, latest.Report.Changed)
	require.Empty(t, failures)
	_, _, recent, detailedFailures := observer.SnapshotDetailed()
	require.Len(t, recent, 2)
	require.Empty(t, detailedFailures)
	require.Equal(t, requestOperationsBefore+1, testutil.ToFloat64(metrics.FailoverVersionTranslationOperations.WithLabelValues("local_to_remote", "success", "none")))
	require.Equal(t, responseOperationsBefore+1, testutil.ToFloat64(metrics.FailoverVersionTranslationOperations.WithLabelValues("remote_to_local", "success", "none")))
}

func TestSemanticStreamCommitsSuccessfulClones(t *testing.T) {
	t.Parallel()
	forward, inverse := testFailoverTranslator(t)
	interceptor := NewSemanticTranslationInterceptor(log.NewTestLogger(), []SemanticTranslationPair{{
		Requests: forward, Responses: inverse,
	}})
	underlying := &semanticCaptureServerStream{
		received: &adminservicepb.DescribeClusterResponse{FailoverVersionIncrement: 105, InitialFailoverVersion: 1},
	}
	stream := &semanticStreamTranslator{ServerStream: underlying, interceptor: interceptor}

	received := &adminservicepb.DescribeClusterResponse{}
	require.NoError(t, stream.RecvMsg(received))
	require.Equal(t, int64(100), received.GetFailoverVersionIncrement())

	sent := &adminservicepb.DescribeClusterResponse{FailoverVersionIncrement: 100, InitialFailoverVersion: 1}
	require.NoError(t, stream.SendMsg(sent))
	require.Equal(t, int64(100), sent.GetFailoverVersionIncrement(), "send translation must not mutate caller input")
	require.Equal(t, int64(105), underlying.sent.(*adminservicepb.DescribeClusterResponse).GetFailoverVersionIncrement())
}

func TestTranslationDebugObserverBoundsFailures(t *testing.T) {
	t.Parallel()
	observer := NewTranslationDebugObserver(2)
	for index := 0; index < 3; index++ {
		observer.Observe(TranslationObservation{
			Outcome: "error", ErrorCode: failoverversion.ErrorUnknownLane, FieldPath: "safe.path",
		})
	}
	latest, lastSuccess, failures := observer.Snapshot()
	require.NotNil(t, latest)
	require.Nil(t, lastSuccess)
	require.Len(t, failures, 2)
	_, _, recent, detailedFailures := observer.SnapshotDetailed()
	require.Len(t, recent, 2)
	require.Len(t, detailedFailures, 2)
}

func TestTranslationDebugObserverRetainsOperationSummariesBeyondRing(t *testing.T) {
	t.Parallel()
	observer := NewTranslationDebugObserver(1)
	observer.Observe(TranslationObservation{
		Direction: failoverversion.RemoteToLocal, Operation: "GetNamespaceReplicationMessagesRequest",
		Outcome: "success", ObservedAt: time.Unix(1, 0),
	})
	observer.Observe(TranslationObservation{
		Direction: failoverversion.RemoteToLocal, Operation: "GetNamespaceReplicationMessagesRequest",
		Outcome: "success", Report: TranslationReport{Changed: true}, ObservedAt: time.Unix(2, 0),
	})
	observer.Observe(TranslationObservation{
		Direction: failoverversion.LocalToRemote, Operation: "PollWorkflowTaskQueueRequest",
		Outcome: "error", ObservedAt: time.Unix(3, 0),
	})

	_, _, recent, _ := observer.SnapshotDetailed()
	require.Len(t, recent, 1)
	require.Equal(t, []TranslationOperationSummary{
		{Direction: failoverversion.LocalToRemote, Operation: "PollWorkflowTaskQueueRequest", Errors: 1, LastOutcome: "error", LastObservedAt: time.Unix(3, 0)},
		{Direction: failoverversion.RemoteToLocal, Operation: "GetNamespaceReplicationMessagesRequest", Successes: 2, Changed: 1, LastOutcome: "success", LastObservedAt: time.Unix(2, 0)},
	}, observer.OperationSummaries())
}
