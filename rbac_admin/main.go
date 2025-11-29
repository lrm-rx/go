package main

import (
	"rbac.admin/core"
	"rbac.admin/flags"
	"rbac.admin/global"
)

func main() {
	core.InitLogger()
	global.Config = core.ReadConfig()
	global.DB = core.InitGorm()
	global.Redis = core.InitRedis()
	// 命令行参数运行
	flags.Run()
	// 运行web服务
}
