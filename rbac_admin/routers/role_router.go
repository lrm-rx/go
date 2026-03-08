package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
	"rbac.admin/api/role_api"
	"rbac.admin/middleware"
)

func RoleRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("role")
	app := api.App.RoleAPI
	g.POST("create", middleware.AuthMiddleware, middleware.BindJson[role_api.RoleCreateRequest], app.RoleCreateView)
}
