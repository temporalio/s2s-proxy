package interceptor

import (
	"strings"

	"go.temporal.io/server/api/adminservice/v1"
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
	v := MakeSearchAttributeVisitor(s.getNamespaceReqMatcher)
	return v.Visit(req)
}

func (s *saTranslator) TranslateResponse(req, resp any) (bool, error) {
	// Detect namespace id in GetWorkflowExecutionRawHistoryV2Request.
	// Use that namespace id to translate search attributes in the response type.
	v := MakeSearchAttributeVisitor(s.getNamespaceRespMatcher)
	switch val := req.(type) {
	case *adminservice.GetWorkflowExecutionRawHistoryV2Request:
		v.currentNamespaceId = val.NamespaceId
	}
	return v.Visit(resp)
}

func (s *saTranslator) getNamespaceReqMatcher(namespaceId string) stringMatcher {
	return s.reqMap[namespaceId]
}

func (s *saTranslator) getNamespaceRespMatcher(namespaceId string) stringMatcher {
	return s.respMap[namespaceId]
}

func createStringMatchers(nsMappings map[string]map[string]string) map[string]stringMatcher {
	result := make(map[string]stringMatcher, len(nsMappings))
	for nsId, mapping := range nsMappings {
		result[nsId] = createStringMatcher(mapping)
	}
	return result
}
