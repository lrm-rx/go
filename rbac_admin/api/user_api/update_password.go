package user_api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
)

func (UserAPI) UpdatePasswordView(c *gin.Context) {
	claims, ok := c.Get("claims")
	fmt.Println(claims, ok)
	res.OkWidthMsg("修改密码成功", c)
}
