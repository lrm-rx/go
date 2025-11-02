package api

import (
	"ContentSystem/internal/services"
	"github.com/gin-gonic/gin"
)

const (
	rootPath   = "/api"
	noAuthPath = "/out/api"
)

func CmsRouters(r *gin.Engine) {
	cmsApp := services.NewCmsApp()
	session := &SessionAuth{}
	root := r.Group(rootPath).Use(session.Auth)
	{
		root.GET("/cms/ping", cmsApp.Hello)
	}

	noAuth := r.Group(noAuthPath)
	{
		noAuth.POST("/cms/register", cmsApp.Register)
	}
}
