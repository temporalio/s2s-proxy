package interceptor

type (
	Translator interface {
		MatchMethod(string) bool
		TranslateRequest(any) (bool, error)
		TranslateResponse(any, any) (bool, error)
	}

	translatorImpl struct {
		matchMethod func(string) bool
		reqVisitor  Visitor
		respVisitor Visitor
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

func (n *translatorImpl) TranslateResponse(_, resp any) (bool, error) {
	return n.respVisitor.Visit(resp)
}

func createStringMatcher(mapping map[string]string) stringMatcher {
	return func(name string) (string, bool) {
		newName, ok := mapping[name]
		return newName, ok
	}
}
