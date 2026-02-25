package middleware

import (
	"fmt"
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

func BindQuery[T any](c *gin.Context) {
	var cr T
	err := c.ShouldBindQuery(&cr)
	if err != nil {
		res.FailWithBinding(err, c)
		c.Abort() // 直接拦截返回
		return
	}
	c.Set("request", cr)
}

func GetBind[T any](c *gin.Context) (data T, err error) {
	if val, exists := c.Get("request"); exists {
		if typedVal, ok := val.(T); ok {
			return typedVal, nil
		}
		return data, fmt.Errorf("类型断言失败")
	}
	return data, fmt.Errorf("未找到请求数据")
}
