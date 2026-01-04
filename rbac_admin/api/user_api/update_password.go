package user_api

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
	"rbac.admin/utils/pwd"
)

type UpdatePasswordRequest struct {
	OldPwd string `json:"OldPwd" binding:"required"`
	Pwd    string `json:"Pwd" binding:"required,min=6,max=64"`
	RePwd  string `json:"RePwd" binding:"required,min=6,max=64"`
}

func (UserAPI) UpdatePasswordView(c *gin.Context) {
	cr := middleware.GetBind[UpdatePasswordRequest](c)
	claims := middleware.GetAuth(c)

	var user models.UserModel
	err := global.DB.Take(&user, claims.UserID).Error
	if err != nil {
		res.FailWidthMsg("用户不存在", c)
		return
	}
	if !pwd.ComparePasswords(user.Password, cr.OldPwd) {
		res.FailWidthMsg("原密码错误", c)
		return
	}
	if cr.Pwd != cr.RePwd {
		res.FailWidthMsg("两次密码不一致", c)
		return
	}
	hashPwd := pwd.HashPassword(cr.Pwd)
	err = global.DB.Model(&user).Update("password", hashPwd).Error
	if err != nil {
		logrus.Errorf("修改密码失败: %v", err)
		res.FailWidthMsg("修改密码失败", c)
		return
	}
	res.OkWidthMsg("修改密码成功", c)
}
