package middleware

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
)

func BindJson[T any](c *gin.Context) {
	var cr T
	err := c.ShouldBindJSON(&cr)
	if err != nil {
		res.FailWithBinding(err, c)
		c.Abort() // 直接拦截返回
		return
	}
	c.Set("request", cr)
}

func GetBind[T any](c *gin.Context) (data T) {
	return c.MustGet("request").(T)
}
