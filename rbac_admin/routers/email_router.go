package routers

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/api"
	"rbac.admin/api/email_api"
	"rbac.admin/middleware"
)

func EmailRouter(r *gin.RouterGroup) {
	// 路由分组: 方便加中间件管理
	g := r.Group("")
	app := api.App.EmailAPI
	g.POST("email/send_email", middleware.BindJson[email_api.SendEmailRequest], app.SendEmailView)
}
