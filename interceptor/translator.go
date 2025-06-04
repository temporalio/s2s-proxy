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
		matchReq:  createNameTranslator(reqMap),
		matchResp: createNameTranslator(respMap),
		visitor:   visitNamespace,
	}
}

func (n *translatorImpl) TranslateRequest(req any) (bool, error) {
	return n.visitor(req, n.matchReq)
}

func (n *translatorImpl) TranslateResponse(resp any) (bool, error) {
	return n.visitor(resp, n.matchResp)
}

func createNameTranslator(mapping map[string]string) matcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
