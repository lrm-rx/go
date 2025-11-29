package main

import (
	"rbac.admin/core"
	"rbac.admin/flags"
	"rbac.admin/global"
	"rbac.admin/routers"
)

func main() {
	core.InitLogger()
	global.Config = core.ReadConfig() // 读取配置文件
	global.DB = core.InitGorm()       // 加载数据库
	global.Casbin = core.InitCasbin() // 加载casbin
	global.Redis = core.InitRedis()   // 连接redis
	// 命令行参数运行
	flags.Run()
	// 运行web服务
	routers.Run()
}
