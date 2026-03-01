package menu_api

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

func (MenuAPI) RemoveView(c *gin.Context) {
	cr, err := middleware.GetBind[models.IDListRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}

	var list []models.MenuModel
	global.DB.Find(&list, "id in ?", cr.IDList)
	var count int64
	if len(list) > 0 {
		count = global.DB.Delete(&list).RowsAffected
	}
	msg := fmt.Sprintf("删除菜单成功，共删除%d条数据", count)
	res.FailWithMsg(msg, c)
}
