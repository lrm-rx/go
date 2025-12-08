package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
)

func UserRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("")
	app := api.App.UserAPI
	g.GET("login", app.LoginView)
}
