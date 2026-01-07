package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
	"rbac.admin/api/user_api"
	"rbac.admin/middleware"
)

func UserRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("")
	app := api.App.UserAPI
	g.POST("login", middleware.BindJson[user_api.LoginRequest], app.LoginView)
	g.POST("register", middleware.BindJson[user_api.RegisterRequest], app.RegisterView)
	g.PUT("user/password", middleware.AuthMiddleware, middleware.BindJson[user_api.UpdatePasswordRequest], app.UpdatePasswordView)
	g.PUT("user", middleware.AuthMiddleware, middleware.BindJson[user_api.UpdateUserinfoRequest], app.UpdateUserinfoView)
	g.GET("user/info", middleware.AuthMiddleware, app.UserinfoView)
}
