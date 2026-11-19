package interceptor

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/temporalio/s2s-proxy/common"
	"github.com/temporalio/s2s-proxy/failoverversion"
	"github.com/temporalio/s2s-proxy/metrics"
)

type SemanticTranslator interface {
	Kind() string
	TranslateMessage(proto.Message) (TranslationResult, error)
	TranslateError(error) ErrorTranslationResult
}

type SemanticTranslationPair struct {
	Requests  SemanticTranslator
	Responses SemanticTranslator
}

type SemanticTranslationInterceptor struct {
	logger   log.Logger
	pairs    []SemanticTranslationPair
	observer TranslationObserver
}

type SemanticTranslationOption func(*SemanticTranslationInterceptor)

func WithSemanticTranslationObserver(observer TranslationObserver) SemanticTranslationOption {
	return func(interceptor *SemanticTranslationInterceptor) {
		interceptor.observer = compositeTranslationObserver{interceptor.observer, observer}
	}
}

func NewSemanticTranslationInterceptor(logger log.Logger, pairs []SemanticTranslationPair, options ...SemanticTranslationOption) *SemanticTranslationInterceptor {
	interceptor := &SemanticTranslationInterceptor{logger: logger, pairs: pairs, observer: prometheusTranslationObserver{}}
	for _, option := range options {
		option(interceptor)
	}
	return interceptor
}

func (i *SemanticTranslationInterceptor) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if i.bypass(ctx, info.FullMethod) {
		return handler(ctx, req)
	}

	if request, ok := req.(proto.Message); ok {
		for _, pair := range i.pairs {
			result, err := i.translateMessage(pair.Requests, request, api.MethodName(info.FullMethod)+"Request")
			if err != nil {
				return nil, semanticInternalError(err)
			}
			request = result.Message
		}
		req = request
	}

	resp, handlerErr := handler(ctx, req)
	if handlerErr != nil {
		for idx := len(i.pairs) - 1; idx >= 0; idx-- {
			translated := i.translateError(i.pairs[idx].Responses, handlerErr, api.MethodName(info.FullMethod)+"Error")
			if translated == nil {
				handlerErr = nil
				continue
			}
			var translationErr *failoverversion.TranslationError
			if errors.As(translated, &translationErr) {
				return nil, semanticInternalError(translated)
			}
			handlerErr = translated
		}
		return nil, handlerErr
	}

	response, ok := resp.(proto.Message)
	if !ok || response == nil {
		return resp, nil
	}
	for idx := len(i.pairs) - 1; idx >= 0; idx-- {
		result, err := i.translateMessage(i.pairs[idx].Responses, response, api.MethodName(info.FullMethod)+"Response")
		if err != nil {
			return nil, semanticInternalError(err)
		}
		response = result.Message
	}
	return response, nil
}

func (i *SemanticTranslationInterceptor) InterceptStream(
	srv any,
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	if i.bypass(ss.Context(), info.FullMethod) || common.IsIntraProxy(ss.Context()) {
		return handler(srv, ss)
	}
	return handler(srv, &semanticStreamTranslator{ServerStream: ss, interceptor: i})
}

func (i *SemanticTranslationInterceptor) bypass(ctx context.Context, fullMethod string) bool {
	return len(i.pairs) == 0 || common.IsRequestTranslationDisabled(ctx) ||
		(!strings.HasPrefix(fullMethod, api.WorkflowServicePrefix) && !strings.HasPrefix(fullMethod, api.AdminServicePrefix))
}

func (i *SemanticTranslationInterceptor) translateMessage(translator SemanticTranslator, message proto.Message, operation string) (TranslationResult, error) {
	if translator == nil {
		return TranslationResult{Message: message}, nil
	}
	start := time.Now()
	result, err := translator.TranslateMessage(message)
	duration := time.Since(start)
	messageType := metrics.SanitizedTypeName(message)
	metrics.TranslationLatency.WithLabelValues(translator.Kind(), messageType).Observe(duration.Seconds())
	if err != nil {
		i.logError(translator, err, operation, messageType)
		i.observe(translator, operation, messageType, "error", err, TranslationReport{}, duration, message)
		return TranslationResult{}, err
	}
	if result.Report.Changed {
		metrics.TranslationCount.WithLabelValues(translator.Kind(), messageType).Inc()
		i.logger.Debug("semantic translation applied", tag.NewStringTag("method", operation), tag.NewStringTag("type", messageType))
	}
	i.observe(translator, operation, messageType, "success", nil, result.Report, duration, result.Message)
	return result, nil
}

func (i *SemanticTranslationInterceptor) translateError(translator SemanticTranslator, input error, operation string) error {
	if translator == nil {
		return input
	}
	start := time.Now()
	result := translator.TranslateError(input)
	translated, report := result.Error, result.Report
	duration := time.Since(start)
	metrics.TranslationLatency.WithLabelValues(translator.Kind(), "grpc.status").Observe(duration.Seconds())
	var translationErr *failoverversion.TranslationError
	if errors.As(translated, &translationErr) {
		i.logError(translator, translated, operation, "grpc.status")
		i.observe(translator, operation, "grpc.status", "error", translated, TranslationReport{}, duration, nil)
		return translated
	}
	if report.Changed {
		metrics.TranslationCount.WithLabelValues(translator.Kind(), "grpc.status").Inc()
	}
	i.observe(translator, operation, "grpc.status", "success", nil, report, duration, nil)
	return translated
}

type directionalSemanticTranslator interface {
	Direction() failoverversion.Direction
}

type describedSemanticTranslator interface {
	MappingDescriptor() failoverversion.MappingDescriptor
}

func (i *SemanticTranslationInterceptor) observe(
	translator SemanticTranslator,
	operation string,
	messageType string,
	outcome string,
	err error,
	report TranslationReport,
	duration time.Duration,
	message proto.Message,
) {
	if i.observer == nil {
		return
	}
	observation := TranslationObservation{
		Kind: translator.Kind(), Operation: operation, MessageType: messageType,
		Outcome: outcome, Report: report, Duration: duration, ObservedAt: time.Now(),
		NamespaceReplication: observeNamespaceReplication(message),
	}
	if directional, ok := translator.(directionalSemanticTranslator); ok {
		observation.Direction = directional.Direction()
	}
	if described, ok := translator.(describedSemanticTranslator); ok {
		observation.MappingFingerprint = described.MappingDescriptor().Fingerprint
	}
	var translationErr *failoverversion.TranslationError
	if errors.As(err, &translationErr) {
		observation.ErrorCode = translationErr.Code
		observation.FieldPath = translationErr.FieldPath
	}
	i.observer.Observe(observation)
}

func (i *SemanticTranslationInterceptor) logError(translator SemanticTranslator, err error, operation, messageType string) {
	metrics.TranslationErrors.WithLabelValues(translator.Kind(), messageType).Inc()
	logTags := []tag.Tag{
		tag.NewStringTag("method", operation),
		tag.NewStringTag("type", messageType),
	}
	var translationErr *failoverversion.TranslationError
	if errors.As(err, &translationErr) {
		logTags = append(logTags,
			tag.NewStringTag("reason", string(translationErr.Code)),
			tag.NewStringTag("direction", string(translationErr.Direction)),
			tag.NewStringTag("field", translationErr.FieldPath),
		)
	}
	i.logger.Error("semantic translation error", logTags...)
}

func semanticInternalError(err error) error {
	var translationErr *failoverversion.TranslationError
	if errors.As(err, &translationErr) {
		return status.Error(codes.Internal, translationErr.Error())
	}
	return status.Error(codes.Internal, "semantic translation failed")
}

type semanticStreamTranslator struct {
	grpc.ServerStream
	interceptor *SemanticTranslationInterceptor
}

func (w *semanticStreamTranslator) RecvMsg(message any) error {
	if err := w.ServerStream.RecvMsg(message); err != nil {
		return err
	}
	protobuf, ok := message.(proto.Message)
	if !ok {
		return nil
	}
	for _, pair := range w.interceptor.pairs {
		result, err := w.interceptor.translateMessage(pair.Requests, protobuf, "RecvMsg")
		if err != nil {
			return semanticInternalError(err)
		}
		protobuf = result.Message
	}
	messageValue := message.(proto.Message)
	if protobuf != messageValue {
		proto.Reset(messageValue)
		proto.Merge(messageValue, protobuf)
	}
	return nil
}

func (w *semanticStreamTranslator) SendMsg(message any) error {
	protobuf, ok := message.(proto.Message)
	if ok {
		for idx := len(w.interceptor.pairs) - 1; idx >= 0; idx-- {
			result, err := w.interceptor.translateMessage(w.interceptor.pairs[idx].Responses, protobuf, "SendMsg")
			if err != nil {
				return semanticInternalError(err)
			}
			protobuf = result.Message
		}
		message = protobuf
	}
	return w.ServerStream.SendMsg(message)
}
