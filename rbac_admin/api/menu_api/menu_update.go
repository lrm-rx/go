package menu_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
	"rbac.admin/utils/maps"
)

type MenuUpdateRequest struct {
	ID           uint    `json:"id" binding:"required"`
	Icon         *string `json:"icon" maps:"icon"`
	Title        *string `json:"title" maps:"title"`
	Enable       *bool   `json:"enable" maps:"enable"` // 是否显示
	Name         *string `json:"name" maps:"name"`
	Path         *string `json:"path" maps:"path"`
	Component    *string `json:"component" maps:"component"`
	ParentMenuID *uint   `json:"parentMenuID" maps:"-"` // 父菜单id
	Sort         *int    `json:"sort" maps:"sort"`
}

func (MenuAPI) MenuUpdateView(c *gin.Context) {
	cr, err := middleware.GetBind[MenuUpdateRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}
	var menu models.MenuModel
	err = global.DB.Take(&menu, cr.ID).Error
	if err != nil {
		res.FailWithMsg("菜单不存在", c)
		return
	}
	// 需要实现一个方法，如果这个字段传了，那就把值放到map里面去
	mps := maps.StructToMaps(cr, "maps")
	if len(mps) == 0 {
		res.FailWithMsg("请求参数绑定失败: 没有可更新的字段", c)
		return
	}
	title, ok := mps["title"]
	if ok {
		if title == "" {
			res.FailWithMsg("菜单title不能为空！", c)
			return
		}
	}
	name, ok := mps["name"]
	if ok {
		if name == "" {
			res.FailWithMsg("菜单name不能为空！", c)
			return
		}
		var model models.MenuModel
		err = global.DB.Not("id = ?", menu.ID).Take(&model, "name = ?", name).Error
		if err != nil {
			res.FailWithMsg("菜单name不能重复！", c)
			return
		}
	}
	path, ok := mps["path"]
	if ok {
		if path == "" {
			res.FailWithMsg("菜单path不能为空！", c)
			return
		}
	}

	if cr.ParentMenuID == nil {
		mps["parent_menu_id"] = nil
	} else {
		parentMenuID := *cr.ParentMenuID
		mps["parent_menu_id"] = parentMenuID
		// 父菜单必须存在
		var model models.MenuModel
		err = global.DB.Take(&model, parentMenuID).Error
		if err != nil {
			res.FailWithMsg("父菜单不存在", c)
			return
		}
		// 不能是自己的所有子菜单
		subMenuList := models.FindSubMenuList(model)
		for _, menuModel := range subMenuList {
			if menuModel.ID == parentMenuID {
				res.FailWithMsg("菜单的父菜单不能是自己或者自己的子菜单！", c)
				return
			}
		}
	}

	err = global.DB.Model(&menu).Updates(mps).Error
	if err != nil {
		res.FailWithMsg("菜单更新失败: "+err.Error(), c)
		return
	}
	res.Ok(menu.ID, "菜单更新成功", c)
}
