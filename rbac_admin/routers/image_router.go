package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
	"rbac.admin/middleware"
)

func ImageRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("image").Use(middleware.AuthMiddleware)
	app := api.App.ImageAPI
	g.POST("upload", app.UploadView)
}
