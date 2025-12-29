package email

type emailStore struct {
	Email string
	Code  string
}

var EmailStoreMap = map[string]emailStore{}

func Set(emailID, email, code string) {
	EmailStoreMap[emailID] = emailStore{
		Email: email,
		Code:  code,
	}
}

func Verify(emailID, email, code string) bool {
	info, ok := EmailStoreMap[emailID]
	if !ok {
		return false
	}
	if info.Email != email {
		return false
	}
	if info.Code != code {
		return false
	}
	return true
}

func Remove(emailID string) {
	delete(EmailStoreMap, emailID)
}
