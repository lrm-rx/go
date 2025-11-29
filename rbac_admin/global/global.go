package global

import (
	"gorm.io/gorm"
	"rbac.admin/config"
)

var (
	Config *config.Config
	DB     *gorm.DB
)
