package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"rbac.admin/core"
	"rbac.admin/global"
)

func main() {
	core.InitLogger()
	global.Config = core.ReadConfig()
	fmt.Println(global.Config)
	logrus.Infof("info")
	logrus.Warnf("warn")
	logrus.Error("error")
}
