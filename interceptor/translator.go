package interceptor

import "go.temporal.io/server/common/log"

type (
	Translator interface {
		MatchMethod(string) bool
		TranslateRequest(any) (bool, error)
		TranslateResponse(any) (bool, error)
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
		kind:        "namespace",
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

func (n *translatorImpl) TranslateResponse(resp any) (bool, error) {
	return n.visitor(n.logger, resp, n.matchResp)
}

func createStringMatcher(mapping map[string]string) stringMatcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
