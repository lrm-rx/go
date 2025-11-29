package global

import (
	"github.com/casbin/casbin/v2"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"rbac.admin/config"
)

var (
	Config *config.Config
	DB     *gorm.DB
	Redis  *redis.Client
	Casbin *casbin.CachedEnforcer
)
