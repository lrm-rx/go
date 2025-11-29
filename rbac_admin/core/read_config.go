package core

import (
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
	"rbac.admin/config"
	"rbac.admin/flags"
)

func ReadConfig() *config.Config {
	byteData, err := os.ReadFile(flags.FlagOptions.File)
	if err != nil {
		//panic("配置文件读取失败" + err.Error())
		//return nil
		logrus.Fatalf("配置文件读取失败 %s", err)
	}
	var c *config.Config
	err = yaml.Unmarshal(byteData, &c)
	if err != nil {
		//panic("配置文件格式错误" + err.Error())
		//return nil
		logrus.Fatalf("配置文件格式错误 %s", err)
	}
	logrus.Infof("配置文件读取成功", flags.FlagOptions.File)
	return c
}
