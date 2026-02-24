package middleware

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/service/redis_service/token_black"
	"rbac.admin/utils/jwts"
)

func AuthMiddleware(c *gin.Context) {
	token := c.GetHeader("token")
	claims, err := jwts.ParseToken(token)
	if err != nil {
		c.Abort()
		res.FailWithMsg("请登录", c)
		return
	}
	// 判断这个token是否在黑名单中
	if token_black.HaveToken(token) {
		res.FailWithMsg("该用户已经注销", c)
		return
	}
	c.Set("claims", claims)
	return
}

func GetAuth(c *gin.Context) *jwts.Claims {
	return c.MustGet("claims").(*jwts.Claims)
}
