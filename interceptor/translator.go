package interceptor

import (
	"strings"

	"go.temporal.io/server/common/api"
)

type (
	Translator interface {
		MatchMethod(string) bool
		TranslateRequest(any) (bool, error)
		TranslateResponse(any) (bool, error)
	}

	translatorImpl struct {
		matchMethod func(string) bool
		matchReq    stringMatcher
		matchResp   stringMatcher
		visitor     visitor
	}
)

func NewNamespaceNameTranslator(reqMap, respMap map[string]string) Translator {
	return &translatorImpl{
		matchMethod: func(string) bool { return true },
		matchReq:    createStringMatcher(reqMap),
		matchResp:   createStringMatcher(respMap),
		visitor:     visitNamespace,
	}
}

func NewSearchAttributeTranslator(reqMap, respMap map[string]string) Translator {
	return &translatorImpl{
		matchMethod: func(method string) bool {
			return !strings.HasPrefix(method, api.WorkflowServicePrefix)
		},
		matchReq:  createStringMatcher(reqMap),
		matchResp: createStringMatcher(respMap),
		visitor:   visitSearchAttributes,
	}
}

func (n *translatorImpl) MatchMethod(m string) bool {
	return n.matchMethod(m)
}

func (n *translatorImpl) TranslateRequest(req any) (bool, error) {
	return n.visitor(req, n.matchReq)
}

func (n *translatorImpl) TranslateResponse(resp any) (bool, error) {
	return n.visitor(resp, n.matchResp)
}

func createStringMatcher(mapping map[string]string) stringMatcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
