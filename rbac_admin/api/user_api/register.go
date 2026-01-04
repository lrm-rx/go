package user_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
	"rbac.admin/utils/email"
	"rbac.admin/utils/pwd"
)

type RegisterRequest struct {
	EmailID    string `json:"emailID" binding:"required"`
	EmailCode  string `json:"emailCode" binding:"required"`
	Email      string `json:"email" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RePassword string `json:"rePassword" binding:"required"`
}

func (UserAPI) RegisterView(c *gin.Context) {
	cr := middleware.GetBind[RegisterRequest](c)

	if !email.Verify(cr.EmailID, cr.Email, cr.EmailCode) {
		res.FailWidthMsg("邮箱验证失败", c)
		return
	}
	// 判断两次密码是否一致
	if cr.Password != cr.RePassword {
		res.FailWidthMsg("两次输入的密码不一致", c)
		return
	}
	// 判断这个邮箱是否已经被注册
	var user models.UserModel
	err := global.DB.Take(&user, "email = ?", cr.Email).Error
	if err == nil {
		res.FailWidthMsg("该邮箱已经被注册", c)
		// 把之前的那个邮箱id删除
		email.Remove(cr.EmailID)
		return
	}
	hashPwd := pwd.HashPassword(cr.RePassword)
	err = global.DB.Create(&models.UserModel{
		Username: cr.Email,
		Email:    cr.Email,
		Password: hashPwd,
	}).Error
	if err != nil {
		res.FailWidthMsg("注册失败", c)
		return
	}
	res.OkWidthMsg("用户注册成功", c)
}
