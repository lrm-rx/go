package routers

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"rbac.admin/global"
)

func Run() {
	s := global.Config.System
	gin.SetMode(s.Mode)
	r := gin.Default()
	g := r.Group("api")
	UserRouter(g)
	// 将 /uploads 的路由映射到uploads目录 注意 ./ 不能少
	r.Static("/uploads", "./uploads")
	logrus.Infof("web服务运行在 %s", s.Addr())
	r.Run(s.Addr())
}
