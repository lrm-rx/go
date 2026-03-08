package role_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

type RoleUpdateRequest struct {
	ID    uint   `json:"id" binding:"required"`
	Title string `json:"title" binding:"required,max=16"`
}

func (RoleAPI) RoleUpdateView(c *gin.Context) {
	cr, err := middleware.GetBind[RoleUpdateRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}

	var role models.RoleModel
	if err := global.DB.Take(&role, cr.ID).Error; err != nil {
		res.FailWithMsg("角色不存在", c)
		return
	}
	if cr.Title == role.Title {
		res.FailWithMsg("角色名称未改变", c)
		return
	}
	if err := global.DB.Not("id = ?", cr.ID).Take(&role, "title = ?", cr.Title).Error; err == nil {
		res.FailWithMsg("角色已存在", c)
		return
	}

	if err := global.DB.Model(&role).Update("title", cr.Title).Error; err != nil {
		res.FailWithMsg("角色更新失败", c)
		return
	}
	res.Ok(role.ID, "角色更新成功", c)
}
