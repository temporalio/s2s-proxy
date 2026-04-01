package interceptor

import (
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	Translator interface {
		MatchMethod(string) bool
		TranslateRequest(any) (bool, error)
		TranslateResponse(any) (bool, error)
		Kind() string
	}

	translatorImpl struct {
		logger      log.Logger
		reg         *metrics.Registry
		matchMethod func(string) bool
		matchReq    stringMatcher
		matchResp   stringMatcher
		kind        string
	}
)

func NewNamespaceNameTranslator(logger log.Logger, reg *metrics.Registry, reqMap, respMap map[string]string) Translator {
	return &translatorImpl{
		logger:      logger,
		reg:         reg,
		matchMethod: func(string) bool { return true },
		matchReq:    createStringMatcher(reqMap),
		matchResp:   createStringMatcher(respMap),
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
	return visitNamespaceWithReg(n.logger, n.reg, req, n.matchReq)
}

func (n *translatorImpl) TranslateResponse(resp any) (bool, error) {
	return visitNamespaceWithReg(n.logger, n.reg, resp, n.matchResp)
}

func createStringMatcher(mapping map[string]string) stringMatcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
