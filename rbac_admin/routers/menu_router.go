package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
	"rbac.admin/api/menu_api"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

func MenuRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("menu")
	app := api.App.MenuAPI
	g.POST("create", middleware.AuthMiddleware, middleware.BindJson[menu_api.MenuCreateRequest], app.MenuCreateView)
	g.POST("update", middleware.AuthMiddleware, middleware.BindJson[menu_api.MenuUpdateRequest], app.MenuUpdateView)
	g.POST("list", middleware.AuthMiddleware, middleware.BindJson[menu_api.MenuListRequest], app.MenuListView)
	g.GET("tree", middleware.AuthMiddleware, app.MenuTreeView)
	g.POST("delete", middleware.AuthMiddleware, middleware.BindJson[models.IDListRequest], app.RemoveView)
}
