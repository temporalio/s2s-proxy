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

	saTranslator struct {
		matchMethod func(string) bool
		reqMap      map[string]map[string]string
		respMap     map[string]map[string]string
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

func (n *translatorImpl) MatchMethod(m string) bool {
	return n.matchMethod(m)
}

func (n *translatorImpl) TranslateRequest(req any) (bool, error) {
	return n.visitor(req, n.matchReq)
}

func (n *translatorImpl) TranslateResponse(resp any) (bool, error) {
	return n.visitor(resp, n.matchResp)
}

func NewSearchAttributeTranslator(reqMap, respMap map[string]map[string]string) Translator {
	return &saTranslator{
		matchMethod: func(method string) bool {
			// In workflowservice APIs, responses only contain the search attribute alias.
			// We should never translate these responses to the search attribute's indexed field.
			return !strings.HasPrefix(method, api.WorkflowServicePrefix)
		},
		reqMap:  reqMap,
		respMap: respMap,
	}
}

func (s *saTranslator) MatchMethod(m string) bool {
	return s.matchMethod(m)
}

func (s *saTranslator) TranslateRequest(req any) (bool, error) {
	return visitSearchAttributes(req, s.getNamespaceReqMatcher(""))
}

func (s *saTranslator) TranslateResponse(resp any) (bool, error) {
	return visitSearchAttributes(resp, s.getNamespaceRespMatcher(""))
}

func (s *saTranslator) getNamespaceReqMatcher(namespaceId string) stringMatcher {
	// Placeholder: Just return the first one (only support one namespace mapping)
	for _, m := range s.reqMap {
		return createStringMatcher(m)
	}
	return createStringMatcher(nil)
}

func (s *saTranslator) getNamespaceRespMatcher(namespaceId string) stringMatcher {
	// Placeholder: Just return the first one (only support one namespace mappping)
	for _, m := range s.respMap {
		return createStringMatcher(m)
	}
	return createStringMatcher(nil)
}

func createStringMatcher(mapping map[string]string) stringMatcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
