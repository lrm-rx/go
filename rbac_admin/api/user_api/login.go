package user_api

import (
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/models"
	"rbac.admin/utils/jwts"
	"rbac.admin/utils/pwd"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required" label:"用户名"`
	Password string `json:"password" binding:"required" label:"密码"`
}

type LoginResponse struct {
	Token string `json:"token"`
}

func (UserAPI) LoginView(c *gin.Context) {
	var cr LoginRequest
	err := c.ShouldBindJSON(&cr)
	if err != nil {
		//c.JSON(200, gin.H{"code": 1001, "msg": err.Error(), "data": nil})
		res.FailWithBinding(err, c)
		return
	}
	var user models.UserModel
	err = global.DB.Preload("RoleList").Take(&user, "username = ?", cr.Username).Error
	if err != nil {
		//c.JSON(200, gin.H{"code": 1001, "msg": "用户名或密码错误!", "data": nil})
		res.FailWidthMsg("用户名或密码错误!", c)
		return
	}

	if !pwd.ComparePasswords(user.Password, cr.Password) {
		res.FailWidthMsg("用户名或密码错误!", c)
		return
	}
	var roleList []uint
	for _, model := range user.RoleList {
		roleList = append(roleList, model.ID)
	}

	// 生成token
	token, err := jwts.GetToken(jwts.ClaimsUserInfo{
		UserID:   user.ID,
		Username: user.Username,
		RoleList: roleList,
	})

	if err != nil {
		logrus.Errorf("jwt颁发token失败 %s", err)
		res.FailWidthMsg("用户名登录失败!", c)
		return
	}

	//c.JSON(200, gin.H{"code": 0, "msg": "用户名登录成功!", "data": LoginResponse{
	//	Token: token,
	//}})
	res.OkWidthData(LoginResponse{
		Token: token,
	}, c)
	return
}
