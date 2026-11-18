package interceptor

import (
	"strings"

	"go.temporal.io/server/common/api"
	"go.temporal.io/server/common/log"

	"github.com/temporalio/s2s-proxy/metrics"
)

type (
	saTranslator struct {
		logger      log.Logger
		matchMethod func(string) bool
		resolveReq  saMatcherResolver
		resolveResp saMatcherResolver
	}
)

func NewSearchAttributeTranslator(logger log.Logger, reqMap, respMap map[string]map[string]string) Translator {
	return &saTranslator{
		logger: logger,
		matchMethod: func(method string) bool {
			// In workflowservice APIs, responses only contain the search attribute alias.
			// We should never translate these responses to the search attribute's indexed field.
			return !strings.HasPrefix(method, api.WorkflowServicePrefix)
		},
		resolveReq:  newSingleMappingResolver(reqMap),
		resolveResp: newSingleMappingResolver(respMap),
	}
}

func (s *saTranslator) Kind() string {
	return metrics.SearchAttrTranslationKind
}

func (s *saTranslator) MatchMethod(m string) bool {
	return s.matchMethod(m)
}

func (s *saTranslator) TranslateRequest(req any) (bool, error) {
	return visitSearchAttributes(s.logger, req, s.resolveReq, "")
}

func (s *saTranslator) TranslateResponse(resp any) (bool, error) {
	return visitSearchAttributes(s.logger, resp, s.resolveResp, "")
}

// newSingleMappingResolver applies one mapping to every namespace, whatever namespace id the
// visitor resolves. The proxy does not start with more than one namespace mapping, so the first
// mapping is the only one. With no mapping, it returns a matcher that matches nothing.
func newSingleMappingResolver(nsMappings map[string]map[string]string) saMatcherResolver {
	match := createStringMatcher(nil)
	for _, matcher := range createStringMatchers(nsMappings) {
		match = matcher
		break
	}
	return func(string) (stringMatcher, bool) {
		return match, true
	}
}

func createStringMatchers(nsMappings map[string]map[string]string) map[string]stringMatcher {
	result := make(map[string]stringMatcher, len(nsMappings))
	for nsId, mapping := range nsMappings {
		result[nsId] = createStringMatcher(mapping)
	}
	return result
}
