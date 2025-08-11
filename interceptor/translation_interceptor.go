package interceptor

import (
	"context"
	"fmt"
	"strings"

	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"
	"go.temporal.io/server/common/log/tag"
	"google.golang.org/grpc"
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
		i.logger.Debug("intercepted request", tag.NewStringTag("method", methodName))

		for _, tr := range i.translators {
			if tr.MatchMethod(info.FullMethod) {
				changed, trErr := tr.TranslateRequest(req)
				logTranslateResult(i.logger, changed, trErr, methodName+"Request", req)
			}
		}

		resp, err := handler(ctx, req)

		for _, tr := range i.translators {
			if tr.MatchMethod(info.FullMethod) {
				changed, trErr := tr.TranslateResponse(resp)
				logTranslateResult(i.logger, changed, trErr, methodName+"Response", resp)
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
	i.logger.Debug("InterceptStream", tag.NewAnyTag("method", info.FullMethod))
	err := handler(srv, newStreamTranslator(ss, i.logger, i.translators))
	if err != nil {
		i.logger.Error("grpc handler with error: %v", tag.Error(err))
	}
	return err
}

type streamTranslator struct {
	grpc.ServerStream
	logger      log.Logger
	translators []Translator
}

func (w *streamTranslator) RecvMsg(m any) error {
	for range 10000 {
		w.logger.Info("spam interceptor")
	}

	w.logger.Debug("Intercept RecvMsg", tag.NewAnyTag("message", m))
	for _, tr := range w.translators {
		changed, trErr := tr.TranslateRequest(m)
		logTranslateResult(w.logger, changed, trErr, "RecvMsg", m)
	}
	return w.ServerStream.RecvMsg(m)
}

func (w *streamTranslator) SendMsg(m any) error {
	w.logger.Debug("Intercept SendMsg", tag.NewStringTag("type", fmt.Sprintf("%T", m)), tag.NewAnyTag("message", m))
	for _, tr := range w.translators {
		changed, trErr := tr.TranslateResponse(m)
		logTranslateResult(w.logger, changed, trErr, "SendMsg", m)
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

func logTranslateResult(logger log.Logger, changed bool, err error, methodName string, obj any) {
	logger = log.With(
		logger,
		tag.NewStringTag("method", methodName),
		tag.NewAnyTag("obj", obj),
	)
	if err != nil {
		logger.Error("translation error", tag.Error(err))
	} else if changed {
		logger.Debug("translation applied")
	} else {
		logger.Debug("translation not applied")
	}
}
