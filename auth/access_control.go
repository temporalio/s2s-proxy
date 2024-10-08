package auth

type (
	AccessControl struct {
		AllowedList []string
	}
)

func NewAccesControl(allowedList []string) *AccessControl {
	return &AccessControl{
		AllowedList: allowedList,
	}
}

func (a *AccessControl) IsAllowed(name string) bool {
	if len(a.AllowedList) == 0 {
		return true
	}

	for _, allowed := range a.AllowedList {
		if allowed == name {
			return true
		}
	}

	return false
}
