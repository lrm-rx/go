package user_api

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

type UpdateUserinfoRequest struct {
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
}

func (UserAPI) UpdateUserinfoView(c *gin.Context) {
	cr, err := middleware.GetBind[UpdateUserinfoRequest](c)
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

	err = global.DB.Model(&user).Updates(models.UserModel{
		Nickname: cr.Nickname,
		Avatar:   cr.Avatar,
	}).Error
	if err != nil {
		logrus.Errorf("修改用户信息失败: %v", err)
		res.FailWithMsg("修改用户信息失败", c)
		return
	}
	res.OkWithMsg("修改用户信息成功", c)
}
