package interceptor

import (
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	Translator interface {
		MatchMethod(string) bool
		TranslateRequest(any) (bool, error)
		// TranslateResponse receives the request paired with resp, so that a translator can
		// take context from it that the response itself does not carry. req is nil for
		// streams, whose messages have no request to pair with.
		TranslateResponse(req, resp any) (bool, error)
		Kind() string
	}

	translatorImpl struct {
		logger      log.Logger
		matchMethod func(string) bool
		matchReq    stringMatcher
		matchResp   stringMatcher
		visitor     visitor
		kind        string
	}
)

func NewNamespaceNameTranslator(logger log.Logger, reqMap, respMap map[string]string) Translator {
	return &translatorImpl{
		logger:      logger,
		matchMethod: func(string) bool { return true },
		matchReq:    createStringMatcher(reqMap),
		matchResp:   createStringMatcher(respMap),
		visitor:     visitNamespace,
		kind:        metrics.NamespaceTranslationKind,
	}
}

func (n *translatorImpl) Kind() string {
	return n.kind
}

func (n *translatorImpl) MatchMethod(m string) bool {
	return n.matchMethod(m)
}

func (n *translatorImpl) TranslateRequest(req any) (bool, error) {
	return n.visitor(n.logger, req, n.matchReq)
}

func (n *translatorImpl) TranslateResponse(_, resp any) (bool, error) {
	return n.visitor(n.logger, resp, n.matchResp)
}

func createStringMatcher(mapping map[string]string) stringMatcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
