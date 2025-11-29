package flags

import (
	"github.com/sirupsen/logrus"
	"rbac.admin/global"
	"rbac.admin/models"
)

func AutoMigrate() {
	err := global.DB.AutoMigrate(
		&models.UserModel{},
		&models.RoleModel{},
		&models.UserRoleModel{},
		&models.MenuModel{},
		&models.ApiModel{},
		&models.RoleMenuModel{},
	)
	if err != nil {
		logrus.Fatalf("表结构迁移失败 %s", err)
	}
	logrus.Infof("表结构迁移成功")
}
