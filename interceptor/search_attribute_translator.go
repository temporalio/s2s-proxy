package interceptor

import (
	"strings"

	"go.temporal.io/server/common/api"
)

type (
	saTranslator struct {
		matchMethod func(string) bool
		reqMap      map[string]stringMatcher
		respMap     map[string]stringMatcher
	}
)

func NewSearchAttributeTranslator(reqMap, respMap map[string]map[string]string) Translator {
	return &saTranslator{
		matchMethod: func(method string) bool {
			// In workflowservice APIs, responses only contain the search attribute alias.
			// We should never translate these responses to the search attribute's indexed field.
			return !strings.HasPrefix(method, api.WorkflowServicePrefix)
		},
		reqMap:  createStringMatchers(reqMap),
		respMap: createStringMatchers(respMap),
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
	for _, matcher := range s.reqMap {
		return matcher
	}
	return createStringMatcher(nil)
}

func (s *saTranslator) getNamespaceRespMatcher(namespaceId string) stringMatcher {
	// Placeholder: Just return the first one (only support one namespace mappping)
	for _, matcher := range s.respMap {
		return matcher
	}
	return createStringMatcher(nil)
}

func createStringMatchers(nsMappings map[string]map[string]string) map[string]stringMatcher {
	result := make(map[string]stringMatcher, len(nsMappings))
	for nsId, mapping := range nsMappings {
		result[nsId] = createStringMatcher(mapping)
	}
	return result
}
