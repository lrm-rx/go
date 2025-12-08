package api

import "rbac.admin/api/user_api"

type API struct {
	UserAPI user_api.UserAPI
}

var App = new(API)
