package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
)

func CaptchaRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("")
	app := api.App.CaptchaAPI
	g.GET("captcha", app.GenerateCaptchaView)
}
