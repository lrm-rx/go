package main

import (
	"rbac.admin/core"
	"rbac.admin/global"
)

func main() {
	core.InitLogger()
	global.Config = core.ReadConfig()
	global.DB = core.InitGorm()
	global.Redis = core.InitRedis()
}
