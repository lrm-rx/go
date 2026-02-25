package user_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/query"
	"rbac.admin/common/res"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

type UserListRequest struct {
	models.Page
	Role     uint   `json:"role"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

type UserListResponse struct {
	models.UserModel
}

func (UserAPI) UserListView(c *gin.Context) {
	cr, err := middleware.GetBind[UserListRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}
	/*
		var userList = make([]models.UserModel, 0)
		offset := (cr.Page.Page - 1) * cr.Limit
		global.DB.Preload("RoleList").Where(models.UserModel{
			Username: cr.Username,
			Email:    cr.Email,
		}).Order(cr.Sort).Where("nickname like ?", fmt.Sprintf("%%%s%%", cr.Key)).Limit(cr.Limit).Offset(offset).Find(&userList)

		var count int64
		global.DB.Model(&models.UserModel{}).Where(models.UserModel{
			Username: cr.Username,
			Email:    cr.Email,
		}).Count(&count)

	*/

	list, count, _ := query.List(models.UserModel{
		Username: cr.Username,
		Email:    cr.Email,
	}, query.Option{
		Page:     cr.Page,
		Debug:    true,
		Likes:    []string{"nickname", "username"},
		Preloads: []string{"RoleList"},
	})

	res.OkWithList(list, count, c)
}
