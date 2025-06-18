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
		reqVisitor  Visitor
		respVisitor Visitor
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
		reqVisitor:  &nsVisitor{match: createStringMatcher(reqMap)},
		respVisitor: &nsVisitor{match: createStringMatcher(respMap)},
	}
}

func (n *translatorImpl) MatchMethod(m string) bool {
	return n.matchMethod(m)
}

func (n *translatorImpl) TranslateRequest(req any) (bool, error) {
	return n.reqVisitor.Visit(req)
}

func (n *translatorImpl) TranslateResponse(resp any) (bool, error) {
	return n.respVisitor.Visit(resp)
}

func NewSearchAttributeTranslator(reqMap, respMap map[string]map[string]string) Translator {
	return &saTranslator{
		matchMethod: func(method string) bool {
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
	v := MakeSearchAttributeVisitor(s.getNamespaceReqMatcher)
	return v.Visit(req)
}

func (s *saTranslator) TranslateResponse(resp any) (bool, error) {
	v := MakeSearchAttributeVisitor(s.getNamespaceRespMatcher)
	return v.Visit(resp)
}

func (s *saTranslator) getNamespaceReqMatcher(namespaceId string) stringMatcher {
	reqMap, ok := s.reqMap[namespaceId]
	if !ok {
		return nil
	}
	return createStringMatcher(reqMap)
}

func (s *saTranslator) getNamespaceRespMatcher(namespaceId string) stringMatcher {
	respMap, ok := s.respMap[namespaceId]
	if !ok {
		return nil
	}
	return createStringMatcher(respMap)
}

func createStringMatcher(mapping map[string]string) stringMatcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
