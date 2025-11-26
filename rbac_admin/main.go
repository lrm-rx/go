package main

import (
	"fmt"
	"rbac.admin/core"
	"rbac.admin/global"
)

func main() {
	global.Config = core.ReadConfig()
	fmt.Println(global.Config)
}
