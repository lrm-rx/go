package user_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

type UserinfoResponse struct {
	UserID   uint   `json:"userID"`
	Nickname string `json:"nickname"`
	Avatar   string `json:"avatar"`
	RoleList []uint `json:"roleList"`
}

func (UserAPI) UserinfoView(c *gin.Context) {
	claims := middleware.GetAuth(c)

	var user models.UserModel
	err := global.DB.Preload("RoleList").Take(&user, claims.UserID).Error

	if err != nil {
		res.FailWidthMsg("用户不存在", c)
		return
	}
	data := UserinfoResponse{
		UserID:   user.ID,
		Nickname: user.Nickname,
		Avatar:   user.Avatar,
		RoleList: user.GetRoleList(),
	}
	res.OkWidthData(data, c)
}
