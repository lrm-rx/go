package menu_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/models"
)

func (MenuAPI) MenuTreeView1(c *gin.Context) {
	var menuList []*models.MenuModel
	global.DB.Order("sort desc").Find(&menuList, "parent_menu_id is null")
	for _, model := range menuList {
		findSubMenuList(model)
	}
	res.OkWithData(menuList, c)
}

func findSubMenuList(model *models.MenuModel) {
	global.DB.Preload("Children").Take(&model)
	for _, child := range model.Children {
		findSubMenuList(child)
	}
}

func (MenuAPI) MenuTreeView(c *gin.Context) {
	// 一次性获取所有菜单
	var allMenus []models.MenuModel
	global.DB.Order("sort desc").Find(&allMenus)

	// 在内存中构建树形结构
	menuMap := make(map[uint]*models.MenuModel)
	var rootMenus []*models.MenuModel

	for i := range allMenus {
		menuMap[allMenus[i].ID] = &allMenus[i]
	}

	for i := range allMenus {
		if allMenus[i].ParentMenuID == nil {
			rootMenus = append(rootMenus, &allMenus[i])
		} else if parent, exists := menuMap[*allMenus[i].ParentMenuID]; exists {
			parent.Children = append(parent.Children, &allMenus[i])
		}
	}

	res.OkWithData(rootMenus, c)
}
