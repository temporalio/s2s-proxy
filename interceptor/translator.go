package interceptor

type (
	Translator interface {
		TranslateRequest(any) (bool, error)
		TranslateResponse(any) (bool, error)
	}

	translatorImpl struct {
		matchReq  matcher
		matchResp matcher
		visitor   visitor
	}
)

func NewNamespaceNameTranslator(reqMap, respMap map[string]string) Translator {
	return &translatorImpl{
		matchReq:  createNameMatcher(reqMap),
		matchResp: createNameMatcher(respMap),
		visitor:   visitNamespace,
	}
}

func NewSearchAttributeTranslator(reqMap, respMap map[string]string) Translator {
	return &translatorImpl{
		matchReq:  createNameMatcher(reqMap),
		matchResp: createNameMatcher(respMap),
		visitor:   visitSearchAttributes,
	}
}

func (n *translatorImpl) TranslateRequest(req any) (bool, error) {
	return n.visitor(req, n.matchReq)
}

func (n *translatorImpl) TranslateResponse(resp any) (bool, error) {
	return n.visitor(resp, n.matchResp)
}

func createNameMatcher(mapping map[string]string) matcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
