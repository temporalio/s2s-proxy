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
// carry history blobs but no namespace field of their own, for example the raw history replies
// and the event batches of SyncWorkflowStateResponse. For these, the NamespaceId of the paired
// request seeds the namespace. A namespace owner inside resp still takes precedence for the
// parts it encloses. req is nil for streams, which have no request to pair with.
//
// This relies on NamespaceId surviving TranslateRequest: the namespace name translator rewrites
// Namespace, never NamespaceId. Adding namespace id translation later would silently break it.
func (s *saTranslator) TranslateResponse(req, resp any) (bool, error) {
	var fallbackNamespaceID string
	if named, ok := req.(interface{ GetNamespaceId() string }); ok {
		fallbackNamespaceID = named.GetNamespaceId()
	}
	return visitSearchAttributes(s.logger, resp, s.resolveResp, fallbackNamespaceID)
}

func newSAMatcherResolver(nsMappings map[string]map[string]string) saMatcherResolver {
	matchers := createStringMatchers(nsMappings)

	return func(nsID string) (stringMatcher, bool) {
		match, ok := matchers[nsID]
		return match, ok
	}
}

func createStringMatchers(nsMappings map[string]map[string]string) map[string]stringMatcher {
	result := make(map[string]stringMatcher, len(nsMappings))
	for nsId, mapping := range nsMappings {
		result[nsId] = createStringMatcher(mapping)
	}
	return result
}
