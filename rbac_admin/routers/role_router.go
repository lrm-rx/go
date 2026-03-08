package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
	"rbac.admin/api/role_api"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

func RoleRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("role")
	app := api.App.RoleAPI
	g.POST("create", middleware.AuthMiddleware, middleware.BindJson[role_api.RoleCreateRequest], app.RoleCreateView)
	g.POST("list", middleware.AuthMiddleware, middleware.BindJson[role_api.RoleListRequest], app.RoleListView)
	g.POST("update", middleware.AuthMiddleware, middleware.BindJson[role_api.RoleUpdateRequest], app.RoleUpdateView)
	g.POST("delete", middleware.AuthMiddleware, middleware.BindJson[models.IDListRequest], app.RemoveView)
}
