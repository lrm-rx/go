package global

import (
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"rbac.admin/config"
)

var (
	Config *config.Config
	DB     *gorm.DB
	Redis  *redis.Client
)
