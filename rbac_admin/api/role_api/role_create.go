package role_api

import (
	"github.com/gin-gonic/gin"
	"rbac.admin/common/res"
	"rbac.admin/global"
	"rbac.admin/middleware"
	"rbac.admin/models"
)

type RoleCreateRequest struct {
	Title string `json:"title" binding:"required,max=16"`
}

func (RoleAPI) RoleCreateView(c *gin.Context) {
	cr, err := middleware.GetBind[RoleCreateRequest](c)
	if err != nil {
		res.FailWithMsg("请求参数绑定失败: "+err.Error(), c)
		return
	}

	var role models.RoleModel
	if err := global.DB.Take(&role, "title = ?", cr.Title).Error; err == nil {
		res.FailWithMsg("角色名称已存在", c)
		return
	}
	role.Title = cr.Title
	if err := global.DB.Create(&role).Error; err != nil {
		res.FailWithMsg("角色创建失败", c)
		return
	}
	res.Ok(role.ID, "角色创建成功", c)
}
