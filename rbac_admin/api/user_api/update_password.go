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
	cr, err := middleware.GetBind[UpdatePasswordRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}
	claims := middleware.GetAuth(c)

	var user models.UserModel
	err = global.DB.Take(&user, claims.UserID).Error
	if err != nil {
		res.FailWithMsg("用户不存在", c)
		return
	}
	if !pwd.ComparePasswords(user.Password, cr.OldPwd) {
		res.FailWithMsg("原密码错误", c)
		return
	}
	if cr.Pwd != cr.RePwd {
		res.FailWithMsg("两次密码不一致", c)
		return
	}
	hashPwd := pwd.HashPassword(cr.Pwd)
	err = global.DB.Model(&user).Update("password", hashPwd).Error
	if err != nil {
		logrus.Errorf("修改密码失败: %v", err)
		res.FailWithMsg("修改密码失败", c)
		return
	}
	res.OkWithMsg("修改密码成功", c)
}
