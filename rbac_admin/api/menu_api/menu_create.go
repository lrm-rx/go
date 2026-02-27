package menu_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

type MenuCreateRequest struct {
	Icon         string `json:"icon"`
	Title        string `json:"title" binding:"required"`
	Enable       bool   `json:"enable"` // 是否显示
	Name         string `json:"name" binding:"required"`
	Path         string `json:"path" binding:"required"`
	Component    string `json:"component"`
	ParentMenuID *uint  `json:"parentMenuID"` // 父菜单id
	Sort         int    `json:"sort"`
}

func (MenuAPI) MenuCreateView(c *gin.Context) {
	cr, err := middleware.GetBind[MenuCreateRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}
	var menu models.MenuModel
	err = global.DB.Take(&menu, "name = ?", cr.Name).Error
	if err == nil {
		res.FailWithMsg("菜单名称已存在", c)
		return
	}
	if cr.ParentMenuID != nil {
		var parent models.MenuModel
		err = global.DB.Take(&parent, *cr.ParentMenuID).Error
		if err != nil {
			res.FailWithMsg("父菜单不存在", c)
			return
		}
	}
	menu = models.MenuModel{
		Enable:       cr.Enable,
		Name:         cr.Name,
		Path:         cr.Path,
		Component:    cr.Component,
		Meta:         models.Meta{Icon: cr.Icon, Title: cr.Title},
		ParentMenuID: cr.ParentMenuID,
		Sort:         cr.Sort,
	}
	err = global.DB.Create(&menu).Error
	if err != nil {
		res.FailWithMsg("菜单创建失败: "+err.Error(), c)
		return
	}
	res.Ok(menu.ID, "菜单创建成功", c)
}
