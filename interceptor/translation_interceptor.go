package interceptor

import (
	"context"
	"strings"
	"time"

	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"

	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	TranslationInterceptor struct {
		logger      log.Logger
		translators []Translator
	}
)

func NewTranslationInterceptor(
	logger log.Logger,
	translators []Translator,
) *TranslationInterceptor {
	return &TranslationInterceptor{
		logger:      logger,
		translators: translators,
	}
}

var _ grpc.UnaryServerInterceptor = (*TranslationInterceptor)(nil).Intercept
var _ grpc.StreamServerInterceptor = (*TranslationInterceptor)(nil).InterceptStream

func (i *TranslationInterceptor) Intercept(
	ctx context.Context,
	req any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	if len(i.translators) > 0 &&
		strings.HasPrefix(info.FullMethod, api.WorkflowServicePrefix) ||
		strings.HasPrefix(info.FullMethod, api.AdminServicePrefix) {

		methodName := api.MethodName(info.FullMethod)

		for _, tr := range i.translators {
			if tr.MatchMethod(info.FullMethod) {
				start := time.Now()
				changed, trErr := tr.TranslateRequest(req)
				logTranslateResult(tr, i.logger, changed, trErr, methodName+"Request", req, time.Since(start))
			}
		}

		resp, err := handler(ctx, req)

		for _, tr := range i.translators {
			if tr.MatchMethod(info.FullMethod) {
				start := time.Now()
				changed, trErr := tr.TranslateResponse(resp)
				logTranslateResult(tr, i.logger, changed, trErr, methodName+"Response", resp, time.Since(start))
			}
		}

		return resp, err
	} else {
		return handler(ctx, req)
	}
}

func (i *TranslationInterceptor) InterceptStream(
	srv any,
	ss grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	return handler(srv, newStreamTranslator(ss, i.logger, i.translators))
}

type streamTranslator struct {
	grpc.ServerStream
	logger      log.Logger
	translators []Translator
}

func (w *streamTranslator) RecvMsg(m any) error {
	for _, tr := range w.translators {
		start := time.Now()
		changed, trErr := tr.TranslateRequest(m)
		logTranslateResult(tr, w.logger, changed, trErr, "RecvMsg", m, time.Since(start))
	}
	return w.ServerStream.RecvMsg(m)
}

func (w *streamTranslator) SendMsg(m any) error {
	for _, tr := range w.translators {
		start := time.Now()
		changed, trErr := tr.TranslateResponse(m)
		logTranslateResult(tr, w.logger, changed, trErr, "SendMsg", m, time.Since(start))
	}
	return w.ServerStream.SendMsg(m)
}

func newStreamTranslator(
	s grpc.ServerStream,
	logger log.Logger,
	translators []Translator,
) grpc.ServerStream {
	return &streamTranslator{
		ServerStream: s,
		logger:       logger,
		translators:  translators,
	}
}

func logTranslateResult(tr Translator, logger log.Logger, changed bool, err error, methodName string, obj any, duration time.Duration) {
	msgType := metrics.SanitizedTypeName(obj)
	metrics.TranslationLatency.WithLabelValues(tr.Kind(), msgType).Observe(duration.Seconds())

	methodTag := tag.NewStringTag("method", methodName)
	if err != nil {
		logger.Error("translation error", methodTag, tag.Error(err), tag.NewStringTag("type", msgType))
		metrics.TranslationErrors.WithLabelValues(tr.Kind(), msgType).Inc()
	} else if changed {
		logger.Debug("translation applied", methodTag, tag.NewAnyTag("obj", obj))
		metrics.TranslationCount.WithLabelValues(tr.Kind(), msgType).Inc()
	} else {
		logger.Debug("translation not applied", methodTag, tag.NewAnyTag("obj", obj))
	}
}
