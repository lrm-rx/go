package role_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/query"
	"rbac.admin/common/res"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

type RoleListRequest struct {
	models.Page
}

type RoleListResponse struct {
	models.Model
	Title         string `json:"title"`
	RoleUserCount int    `json:"roleUserCount"`
	RoleApiCount  int    `json:"roleApiCount"` // todo
	RoleMenuCount int    `json:"roleMenuCount"`
}

func (RoleAPI) RoleListView(c *gin.Context) {
	cr, err := middleware.GetBind[RoleListRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}

	_list, count, _ := query.List(models.RoleModel{}, query.Option{
		Page:     cr.Page,
		Likes:    []string{"title"},
		Preloads: []string{"UserList", "MenuList"},
	})
	var list = make([]RoleListResponse, 0)
	for _, model := range _list {
		list = append(list, RoleListResponse{
			Model:         model.Model,
			Title:         model.Title,
			RoleUserCount: len(model.UserList),
			RoleMenuCount: len(model.MenuList),
		})
	}

	res.OkWithList(list, count, c)
}
