package interceptor

type Translator interface {
	TranslateRequest(any) (bool, error)
	TranslateResponse(any) (bool, error)
}

type translatorImpl struct {
	matchReq  matcher
	matchResp matcher
	visitor   visitorFn
}

func NewNamespaceNameTranslator(reqMap, respMap map[string]string) Translator {
	return &translatorImpl{
		matchReq:  createNameTranslator(reqMap),
		matchResp: createNameTranslator(respMap),
		visitor:   visitNamespace,
	}
}

func NewClusterNameTranslator(reqMap, respMap map[string]string) Translator {
	return &translatorImpl{
		matchReq:  createNameTranslator(reqMap),
		matchResp: createNameTranslator(respMap),
		visitor:   visitClusterName,
	}
}

// TranslateRequest implements Translator.
func (n *translatorImpl) TranslateRequest(req any) (bool, error) {
	return visitNamespace(req, n.matchReq)
}

// TranslateResponse implements Translator.
func (n *translatorImpl) TranslateResponse(resp any) (bool, error) {
	return visitNamespace(resp, n.matchResp)
}

func createNameTranslator(mapping map[string]string) matcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
