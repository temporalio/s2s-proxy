package auth

type (
	AccessControl struct {
		allowedMap map[string]bool
	}
)

func NewAccesControl(allowedList []string) *AccessControl {
	allowedMap := make(map[string]bool)
	for _, allowed := range allowedList {
		allowedMap[allowed] = true
	}
	return &AccessControl{
		allowedMap: allowedMap,
	}
}

func (a *AccessControl) IsAllowed(name string) bool {
	if len(a.allowedMap) == 0 {
		return true
	}

	return a.allowedMap[name]
}
