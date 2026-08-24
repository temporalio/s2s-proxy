package interceptor

import (
	"strings"

	"go.temporal.io/server/api/adminservice/v1"
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
		// The resolvers are built once here and hold no per-message state, so a single
		// translator serves every concurrent stream.
		resolveReq:  newSAMatcherResolver(reqMap),
		resolveResp: newSAMatcherResolver(respMap),
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

// TranslateResponse translates the search attributes in resp. Some admin service responses
// carry history blobs but no namespace field of their own, so the paired request is used to
// seed the namespace. req is nil for streams, which have no request to pair with.
//
// This relies on NamespaceId surviving TranslateRequest: the namespace name translator rewrites
// Namespace, never NamespaceId. Adding namespace id translation later would silently break it.
func (s *saTranslator) TranslateResponse(req, resp any) (bool, error) {
	var fallbackNamespaceID string
	switch r := req.(type) {
	case *adminservice.GetWorkflowExecutionRawHistoryV2Request:
		fallbackNamespaceID = r.NamespaceId
	case *adminservice.GetWorkflowExecutionRawHistoryRequest:
		fallbackNamespaceID = r.NamespaceId
	}
	return visitSearchAttributes(s.logger, resp, s.resolveResp, fallbackNamespaceID)
}

func newSAMatcherResolver(nsMappings map[string]map[string]string) saMatcherResolver {
	matchers := createStringMatchers(nsMappings)

	// Legacy configs express a single mapping keyed by an empty namespace id, meaning "apply to
	// every namespace". Mirrors config.LegacyWildcardNamespaceID.
	wildcard, hasWildcard := matchers[""]

	return func(nsID string) (stringMatcher, bool) {
		if match, ok := matchers[nsID]; ok {
			return match, true
		}
		if hasWildcard {
			return wildcard, true
		}
		return nil, false
	}
}

func createStringMatchers(nsMappings map[string]map[string]string) map[string]stringMatcher {
	result := make(map[string]stringMatcher, len(nsMappings))
	for nsId, mapping := range nsMappings {
		result[nsId] = createStringMatcher(mapping)
	}
	return result
}
