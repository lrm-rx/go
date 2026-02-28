package menu_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/query"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

type MenuListRequest struct {
	models.Page
}

func (MenuAPI) MenuListView(c *gin.Context) {
	cr, err := middleware.GetBind[MenuListRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}

	cr.Page.Sort = "sort desc"

	list, count, _ := query.List(models.MenuModel{}, query.Option{
		Page:     cr.Page,
		Likes:    []string{"name", "title"},
		Where:    global.DB.Where("parent_menu_id is null"),
		Preloads: []string{"Children"},
	})

	res.OkWithList(list, count, c)
}
