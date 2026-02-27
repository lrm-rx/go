package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
	"rbac.admin/api/menu_api"
	"rbac.admin/middleware"
)

func MenuRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("menu")
	app := api.App.MenuAPI
	g.POST("create", middleware.AuthMiddleware, middleware.BindJson[menu_api.MenuCreateRequest], app.MenuCreateView)
}
