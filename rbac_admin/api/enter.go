package api

import (
	"rbac.admin/api/captcha_api"
	"rbac.admin/api/email_api"
	"rbac.admin/api/user_api"
)

type API struct {
	UserAPI    user_api.UserAPI
	CaptchaAPI captcha_api.CaptchaAPI
	EmailAPI   email_api.EmailAPI
}

var App = new(API)
